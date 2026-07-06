package backup

import (
	"context"
	"fmt"
	"io"

	"backpull/internal/config"
	"backpull/internal/store"
)

type CommandRunner interface {
	RunCommand(ctx context.Context, command string, stdout io.Writer) error
}

// DBDump runs the service's dump command via docker exec and streams the
// output into <service>.sql. The command runs bare (no remote pipe) so its
// exit code is preserved.
func DBDump(ctx context.Context, runner CommandRunner, run *store.Run, svc config.Service) error {
	f, err := run.Create(svc.Name, svc.Name+".sql")
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	out := &countingWriter{w: f}

	command := fmt.Sprintf("docker exec %s %s", svc.Container, svc.Command)
	if err := runner.RunCommand(ctx, command, out); err != nil {
		return err
	}
	if out.n == 0 {
		return fmt.Errorf("command %q produced no output", command)
	}
	return f.Commit()
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
