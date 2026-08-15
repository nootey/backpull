package localclient

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"

	"go.uber.org/zap"
)

const maxStderr = 4096

// Client runs job commands on the local machine, mirroring sshclient.Client so
// both satisfy job.CommandRunner.
type Client struct {
	log *zap.Logger
}

func New(log *zap.Logger) *Client {
	return &Client{log: log}
}

func (c *Client) RunCommand(ctx context.Context, command string, stdout io.Writer) error {
	name, args := shell(command)
	cmd := exec.CommandContext(ctx, name, args...)

	var stderr limitedBuffer
	cmd.Stdout = stdout
	cmd.Stderr = &stderr

	c.log.Debug("executing local command", zap.String("command", command))
	err := cmd.Run()
	// a cancelled context kills the process, so the wait error is "signal:
	// killed"; report the context error instead so callers see the timeout
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("command %q: %w: %s", command, err, msg)
		}
		return fmt.Errorf("command %q: %w", command, err)
	}
	// commands like tar report warnings on stderr even when they succeed
	if msg := strings.TrimSpace(stderr.String()); msg != "" {
		c.log.Warn("local command wrote to stderr", zap.String("command", command), zap.String("stderr", msg))
	}
	c.log.Debug("local command finished", zap.String("command", command))
	return nil
}

// shell picks the interpreter for the client OS: job commands are shell
// strings, so a local job is tied to the platform it runs from.
func shell(command string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", command}
	}
	return "sh", []string{"-c", command}
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
