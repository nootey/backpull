package job

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"go.uber.org/zap"

	"backpull/internal/config"
	"backpull/internal/store"
)

type CommandRunner interface {
	RunCommand(ctx context.Context, command string, stdout io.Writer) error
}

// Run executes the job's command verbatim on the remote host and streams its
// stdout into the job's output file.
func Run(ctx context.Context, log *zap.Logger, runner CommandRunner, run *store.Run, j config.Job) error {
	f, err := run.Create(j.Name, j.Output)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	log.Info("running job",
		zap.String("job", j.Name),
		zap.String("command", j.Command),
		zap.String("output", f.Path()),
	)

	timeout := time.Duration(j.Timeout)
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	out := &countingWriter{w: f}
	start := time.Now()
	if err := runner.RunCommand(ctx, j.Command, out); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("timed out after %s", timeout)
		}
		return err
	}
	if out.n == 0 {
		return fmt.Errorf("command %q produced no output", j.Command)
	}
	if err := f.Commit(); err != nil {
		return err
	}

	log.Info("job finished",
		zap.String("job", j.Name),
		zap.Int64("bytes", out.n),
		zap.Duration("duration", time.Since(start).Round(time.Millisecond)),
	)
	return nil
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}
