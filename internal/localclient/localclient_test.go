package localclient

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

// commands differ per shell, so each test picks the form for the client OS
func pick(unix, windows string) string {
	if runtime.GOOS == "windows" {
		return windows
	}
	return unix
}

func TestRunCommand(t *testing.T) {
	var out bytes.Buffer
	c := New(zap.NewNop())

	if err := c.RunCommand(context.Background(), pick("echo hello", "echo hello"), &out); err != nil {
		t.Fatalf("RunCommand returned error: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "hello" {
		t.Errorf("stdout = %q, want %q", got, "hello")
	}
}

func TestRunCommandFailure(t *testing.T) {
	var out bytes.Buffer
	c := New(zap.NewNop())

	err := c.RunCommand(context.Background(), pick("echo boom >&2; exit 1", "echo boom 1>&2 & exit 1"), &out)
	if err == nil {
		t.Fatal("RunCommand succeeded, want error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %q, want it to include stderr %q", err, "boom")
	}
}

// stderr on a successful command is a warning, not a failure (e.g. tar notices)
func TestRunCommandStderrOnSuccess(t *testing.T) {
	var out bytes.Buffer
	c := New(zap.NewNop())

	err := c.RunCommand(context.Background(), pick("echo data; echo notice >&2", "echo data & echo notice 1>&2"), &out)
	if err != nil {
		t.Fatalf("RunCommand returned error: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != "data" {
		t.Errorf("stdout = %q, want %q", got, "data")
	}
}

func TestRunCommandTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	var out bytes.Buffer
	c := New(zap.NewNop())

	err := c.RunCommand(ctx, pick("sleep 10", "ping -n 11 127.0.0.1 >NUL"), &out)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded so job.Run reports a timeout", err)
	}
}

func TestLimitedBufferCaps(t *testing.T) {
	var b limitedBuffer
	n, err := b.Write(bytes.Repeat([]byte("x"), maxStderr+100))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if n != maxStderr+100 {
		t.Errorf("Write reported %d bytes, want the full %d so the command never blocks", n, maxStderr+100)
	}
	if len(b.String()) != maxStderr {
		t.Errorf("buffered %d bytes, want it capped at %d", len(b.String()), maxStderr)
	}
}
