package sshclient

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"backpull/internal/config"
)

type testServer struct {
	addr        string
	keyPath     string
	knownHosts  string
	hostSigners []ssh.Signer
	clientKey   ed25519.PrivateKey
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()
	dir := t.TempDir()

	_, cliPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cliSigner, err := ssh.NewSignerFromKey(cliPriv)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(cliPriv, "")
	if err != nil {
		t.Fatal(err)
	}
	keyPath := filepath.Join(dir, "id_ed25519")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}

	_, hostEd, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	hostEc, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var hostSigners []ssh.Signer
	for _, key := range []any{hostEd, hostEc} {
		signer, err := ssh.NewSignerFromKey(key)
		if err != nil {
			t.Fatal(err)
		}
		hostSigners = append(hostSigners, signer)
	}

	conf := &ssh.ServerConfig{
		PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if bytes.Equal(key.Marshal(), cliSigner.PublicKey().Marshal()) {
				return nil, nil
			}
			return nil, errors.New("unknown public key")
		},
	}
	for _, signer := range hostSigners {
		conf.AddHostKey(signer)
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go serveSSH(l, conf)

	addr := l.Addr().String()
	khPath := filepath.Join(dir, "known_hosts")
	var lines []string
	for _, signer := range hostSigners {
		lines = append(lines, knownhosts.Line([]string{addr}, signer.PublicKey()))
	}
	if err := os.WriteFile(khPath, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	return &testServer{addr: addr, keyPath: keyPath, knownHosts: khPath, hostSigners: hostSigners, clientKey: cliPriv}
}

func serveSSH(l net.Listener, conf *ssh.ServerConfig) {
	for {
		nc, err := l.Accept()
		if err != nil {
			return
		}
		go func() {
			_, chans, reqs, err := ssh.NewServerConn(nc, conf)
			if err != nil {
				return
			}
			go ssh.DiscardRequests(reqs)
			for newCh := range chans {
				if newCh.ChannelType() != "session" {
					_ = newCh.Reject(ssh.UnknownChannelType, "unsupported")
					continue
				}
				ch, sessReqs, err := newCh.Accept()
				if err != nil {
					continue
				}
				go handleSession(ch, sessReqs)
			}
		}()
	}
}

func handleSession(ch ssh.Channel, reqs <-chan *ssh.Request) {
	defer func() { _ = ch.Close() }()
	for req := range reqs {
		if req.Type != "exec" {
			_ = req.Reply(false, nil)
			continue
		}
		var payload struct{ Command string }
		_ = ssh.Unmarshal(req.Payload, &payload)
		_ = req.Reply(true, nil)
		status := runFakeCommand(payload.Command, ch)
		_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
		return
	}
}

func runFakeCommand(cmd string, ch ssh.Channel) uint32 {
	switch {
	case strings.HasPrefix(cmd, "echo "):
		_, _ = io.WriteString(ch, strings.TrimPrefix(cmd, "echo ")+"\n")
		return 0
	case cmd == "fail":
		_, _ = io.WriteString(ch.Stderr(), "boom\n")
		return 3
	case cmd == "hang":
		time.Sleep(30 * time.Second)
		return 0
	}
	return 127
}

func (s *testServer) sshConfig(t *testing.T) config.SSH {
	t.Helper()
	host, portStr, err := net.SplitHostPort(s.addr)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	return config.SSH{Host: host, User: "test", Port: port, KeyFile: s.keyPath}
}

func dialTestServer(t *testing.T, srv *testServer) *Client {
	t.Helper()
	c, err := connect(srv.sshConfig(t), srv.knownHosts)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestRunCommandStreamsStdout(t *testing.T) {
	c := dialTestServer(t, newTestServer(t))

	var out bytes.Buffer
	if err := c.RunCommand(context.Background(), "echo hello", &out); err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	if out.String() != "hello\n" {
		t.Errorf("stdout = %q, want %q", out.String(), "hello\n")
	}
}

func TestRunCommandNonZeroExit(t *testing.T) {
	c := dialTestServer(t, newTestServer(t))

	err := c.RunCommand(context.Background(), "fail", io.Discard)
	if err == nil {
		t.Fatal("RunCommand succeeded, want error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %q, want it to contain stderr %q", err.Error(), "boom")
	}
	if !strings.Contains(err.Error(), "3") {
		t.Errorf("error = %q, want it to contain exit status 3", err.Error())
	}
}

func TestRunCommandContextCancel(t *testing.T) {
	c := dialTestServer(t, newTestServer(t))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.RunCommand(ctx, "hang", io.Discard) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("RunCommand did not return after cancel")
	}
}

func TestConnectSingleKnownHostKeyType(t *testing.T) {
	srv := newTestServer(t)
	for _, signer := range srv.hostSigners {
		alg := signer.PublicKey().Type()
		t.Run(alg, func(t *testing.T) {
			kh := filepath.Join(t.TempDir(), "known_hosts")
			line := knownhosts.Line([]string{srv.addr}, signer.PublicKey())
			if err := os.WriteFile(kh, []byte(line+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}

			c, err := connect(srv.sshConfig(t), kh)
			if err != nil {
				t.Fatalf("connect with only %s in known_hosts: %v", alg, err)
			}
			_ = c.Close()
		})
	}
}

func TestConnectUnknownHost(t *testing.T) {
	srv := newTestServer(t)
	empty := filepath.Join(t.TempDir(), "known_hosts")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := connect(srv.sshConfig(t), empty)
	if err == nil {
		t.Fatal("connect succeeded, want unknown host error")
	}
	if !strings.Contains(err.Error(), "known_hosts") {
		t.Errorf("error = %q, want it to mention known_hosts", err.Error())
	}
}

func TestConnectHostKeyMismatch(t *testing.T) {
	srv := newTestServer(t)

	_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherSigner, err := ssh.NewSignerFromKey(otherPriv)
	if err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(t.TempDir(), "known_hosts")
	line := knownhosts.Line([]string{srv.addr}, otherSigner.PublicKey())
	if err := os.WriteFile(bad, []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = connect(srv.sshConfig(t), bad)
	if err == nil {
		t.Fatal("connect succeeded, want host key mismatch error")
	}
	if !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("error = %q, want it to mention mismatch", err.Error())
	}
}

func writeEncryptedKey(t *testing.T, key ed25519.PrivateKey, passphrase string) string {
	t.Helper()
	block, err := ssh.MarshalPrivateKeyWithPassphrase(key, "", []byte(passphrase))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "id_enc")
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func stubPassphrase(t *testing.T, fn func(path string) ([]byte, error)) {
	t.Helper()
	orig := readPassphrase
	readPassphrase = fn
	t.Cleanup(func() { readPassphrase = orig })
}

func TestConnectPassphrasePrompt(t *testing.T) {
	srv := newTestServer(t)
	cfg := srv.sshConfig(t)
	cfg.KeyFile = writeEncryptedKey(t, srv.clientKey, "secret")

	stubPassphrase(t, func(path string) ([]byte, error) { return []byte("secret"), nil })

	c, err := connect(cfg, srv.knownHosts)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = c.Close() }()

	var out bytes.Buffer
	if err := c.RunCommand(context.Background(), "echo hi", &out); err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	if out.String() != "hi\n" {
		t.Errorf("stdout = %q, want %q", out.String(), "hi\n")
	}
}

func TestConnectWrongPassphrase(t *testing.T) {
	srv := newTestServer(t)
	cfg := srv.sshConfig(t)
	cfg.KeyFile = writeEncryptedKey(t, srv.clientKey, "secret")

	stubPassphrase(t, func(path string) ([]byte, error) { return []byte("wrong"), nil })

	_, err := connect(cfg, srv.knownHosts)
	if err == nil {
		t.Fatal("connect succeeded, want decrypt error")
	}
	if !strings.Contains(err.Error(), "decrypt") {
		t.Errorf("error = %q, want it to mention decrypt", err.Error())
	}
}

func TestConnectPassphrasePromptUnavailable(t *testing.T) {
	srv := newTestServer(t)
	cfg := srv.sshConfig(t)
	cfg.KeyFile = writeEncryptedKey(t, srv.clientKey, "secret")

	stubPassphrase(t, func(path string) ([]byte, error) { return nil, errors.New("no terminal") })

	_, err := connect(cfg, srv.knownHosts)
	if err == nil {
		t.Fatal("connect succeeded, want passphrase error")
	}
	if !strings.Contains(err.Error(), "passphrase") {
		t.Errorf("error = %q, want it to mention passphrase", err.Error())
	}
}

func TestConnectMissingKeyFile(t *testing.T) {
	srv := newTestServer(t)
	cfg := srv.sshConfig(t)
	cfg.KeyFile = filepath.Join(t.TempDir(), "nope")

	_, err := connect(cfg, srv.knownHosts)
	if err == nil {
		t.Fatal("connect succeeded, want missing key error")
	}
}
