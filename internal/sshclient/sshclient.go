package sshclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"

	skeemakh "github.com/skeema/knownhosts"

	"backpull/internal/config"
)

const maxStderr = 4096

type Client struct {
	conn *ssh.Client
	log  *zap.Logger
}

func Connect(log *zap.Logger, cfg config.SSH) (*Client, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return connect(log, cfg, filepath.Join(home, ".ssh", "known_hosts"))
}

func connect(log *zap.Logger, cfg config.SSH, knownHostsPath string) (*Client, error) {
	hostKeys, err := skeemakh.NewDB(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("loading known_hosts: %w", err)
	}

	auth, err := authMethod(cfg.KeyFile)
	if err != nil {
		return nil, err
	}

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	log.Debug("dialing", zap.String("addr", addr), zap.String("user", cfg.User))
	conn, err := ssh.Dial("tcp", addr, &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: hostKeys.HostKeyCallback(),
		// only negotiate key types present in known_hosts, else the server
		// may present a key type we don't have on file → false mismatch
		HostKeyAlgorithms: hostKeys.HostKeyAlgorithms(addr),
		Timeout:           15 * time.Second,
	})
	if err != nil {
		var keyErr *knownhosts.KeyError
		if errors.As(err, &keyErr) {
			if len(keyErr.Want) == 0 {
				return nil, fmt.Errorf("%s is not in known_hosts; connect once with `ssh %s@%s` to add it", addr, cfg.User, cfg.Host)
			}
			return nil, fmt.Errorf("host key mismatch for %s: %w", addr, err)
		}
		return nil, fmt.Errorf("connecting to %s: %w", addr, err)
	}
	return &Client{conn: conn, log: log}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) RunCommand(ctx context.Context, command string, stdout io.Writer) error {
	sess, err := c.conn.NewSession()
	if err != nil {
		return fmt.Errorf("opening session: %w", err)
	}
	defer func() { _ = sess.Close() }()

	var stderr limitedBuffer
	sess.Stdout = stdout
	sess.Stderr = &stderr

	c.log.Debug("executing remote command", zap.String("command", command))
	if err := sess.Start(command); err != nil {
		return fmt.Errorf("command %q: %w", command, err)
	}

	done := make(chan error, 1)
	go func() { done <- sess.Wait() }()

	select {
	case <-ctx.Done():
		_ = sess.Close()
		<-done
		return ctx.Err()
	case err := <-done:
		if err != nil {
			if msg := strings.TrimSpace(stderr.String()); msg != "" {
				return fmt.Errorf("command %q: %w: %s", command, err, msg)
			}
			return fmt.Errorf("command %q: %w", command, err)
		}
		// commands like pg_dump report warnings on stderr even when they succeed
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			c.log.Warn("remote command wrote to stderr", zap.String("command", command), zap.String("stderr", msg))
		}
		c.log.Debug("remote command finished", zap.String("command", command))
		return nil
	}
}

func authMethod(keyFile string) (ssh.AuthMethod, error) {
	if keyFile != "" {
		path := expandHome(keyFile)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading key file: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(data)
		if err != nil {
			var passErr *ssh.PassphraseMissingError
			if !errors.As(err, &passErr) {
				return nil, fmt.Errorf("parsing key %s: %w", path, err)
			}
			pass, err := readPassphrase(path)
			if err != nil {
				return nil, fmt.Errorf("key %s needs a passphrase: %w", path, err)
			}
			signer, err = ssh.ParsePrivateKeyWithPassphrase(data, pass)
			if err != nil {
				return nil, fmt.Errorf("decrypting key %s: %w", path, err)
			}
		}
		return ssh.PublicKeys(signer), nil
	}

	conn, err := dialAgent()
	if err != nil {
		return nil, fmt.Errorf("no key_file set and ssh-agent unavailable: %w", err)
	}
	return ssh.PublicKeysCallback(agent.NewClient(conn).Signers), nil
}

var readPassphrase = promptPassphrase

func promptPassphrase(path string) ([]byte, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return nil, errors.New("no terminal to prompt on; use ssh-agent or an unencrypted key")
	}
	fmt.Fprintf(os.Stderr, "Enter passphrase for %s: ", path)
	defer fmt.Fprintln(os.Stderr)
	return term.ReadPassword(fd)
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}

type limitedBuffer struct {
	buf bytes.Buffer
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := maxStderr - b.buf.Len(); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		b.buf.Write(p)
	}
	return n, nil
}

func (b *limitedBuffer) String() string {
	return b.buf.String()
}
