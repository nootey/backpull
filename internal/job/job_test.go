package job

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"backpull/internal/config"
	"backpull/internal/store"
)

var testTime = time.Date(2026, 7, 6, 15, 30, 12, 0, time.UTC)

var testJob = config.Job{
	Name:    "postgres",
	Command: "docker exec app-db pg_dump -U app appdb",
	Output:  "postgres.sql",
}

type fakeRunner struct {
	gotCommand string
	output     []byte
	err        error
}

func (r *fakeRunner) RunCommand(_ context.Context, command string, stdout io.Writer) error {
	r.gotCommand = command
	if _, err := stdout.Write(r.output); err != nil {
		return err
	}
	return r.err
}

func TestRun(t *testing.T) {
	dest := t.TempDir()
	run := store.NewRun(dest, testTime)
	runner := &fakeRunner{output: []byte("-- PostgreSQL database dump")}

	if err := Run(context.Background(), zap.NewNop(), runner, run, testJob); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if runner.gotCommand != testJob.Command {
		t.Errorf("command = %q, want %q", runner.gotCommand, testJob.Command)
	}

	final := filepath.Join(dest, "2026-07-06_153012", "postgres", "postgres.sql")
	data, err := os.ReadFile(final)
	if err != nil {
		t.Fatalf("reading final file: %v", err)
	}
	if string(data) != "-- PostgreSQL database dump" {
		t.Errorf("output contents = %q, want %q", data, "-- PostgreSQL database dump")
	}
}

func TestRunCommandError(t *testing.T) {
	dest := t.TempDir()
	run := store.NewRun(dest, testTime)
	runner := &fakeRunner{output: []byte("partial output"), err: errors.New("exit status 1")}

	if err := Run(context.Background(), zap.NewNop(), runner, run, testJob); err == nil {
		t.Fatal("Run succeeded, want error")
	}

	dir := filepath.Join(dest, "2026-07-06_153012", "postgres")
	if _, err := os.Stat(filepath.Join(dir, "postgres.sql")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("final file exists after failed command (stat err: %v)", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "postgres.sql.partial")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf(".partial left behind after failed command (stat err: %v)", err)
	}
}

// a job whose output_dir came from {output_path} gets its directories created,
// because output_root existing already proves the drive is mounted
func TestRunCreatesDirsUnderOutputRoot(t *testing.T) {
	root := t.TempDir()
	j := testJob
	j.OutputRoot = root
	j.OutputDir = filepath.Join(root, "notes", "2027")

	runner := &fakeRunner{output: []byte("archive")}
	if err := Run(context.Background(), zap.NewNop(), runner, store.NewRun(t.TempDir(), testTime), j); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(j.OutputDir, j.Output))
	if err != nil {
		t.Fatalf("reading final file: %v", err)
	}
	if string(data) != "archive" {
		t.Errorf("contents = %q, want %q", data, "archive")
	}
}

func TestRunMissingOutputRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "unmounted")
	j := testJob
	j.OutputRoot = root
	j.OutputDir = filepath.Join(root, "notes", "2027")

	runner := &fakeRunner{output: []byte("archive")}
	err := Run(context.Background(), zap.NewNop(), runner, store.NewRun(t.TempDir(), testTime), j)
	if err == nil {
		t.Fatal("Run succeeded with a missing output_root, want error")
	}
	if runner.gotCommand != "" {
		t.Error("command ran despite the drive being unmounted; the output file is created first")
	}
}

// hangingRunner blocks until the context is cancelled, like a hung remote command.
type hangingRunner struct{}

func (hangingRunner) RunCommand(ctx context.Context, _ string, stdout io.Writer) error {
	if _, err := io.WriteString(stdout, "partial output"); err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestRunTimeout(t *testing.T) {
	dest := t.TempDir()
	run := store.NewRun(dest, testTime)
	j := testJob
	j.Timeout = config.Duration(50 * time.Millisecond)

	err := Run(context.Background(), zap.NewNop(), hangingRunner{}, run, j)
	if err == nil {
		t.Fatal("Run succeeded, want timeout error")
	}
	if !strings.Contains(err.Error(), "timed out after 50ms") {
		t.Errorf("error = %q, want it to mention timing out", err)
	}

	final := filepath.Join(dest, "2026-07-06_153012", "postgres", "postgres.sql")
	if _, err := os.Stat(final); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("final file exists after timeout (stat err: %v)", err)
	}
}

func TestRunEmptyOutput(t *testing.T) {
	dest := t.TempDir()
	run := store.NewRun(dest, testTime)
	runner := &fakeRunner{output: nil}

	err := Run(context.Background(), zap.NewNop(), runner, run, testJob)
	if err == nil {
		t.Fatal("Run succeeded on empty output, want error")
	}

	final := filepath.Join(dest, "2026-07-06_153012", "postgres", "postgres.sql")
	if _, err := os.Stat(final); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("final file exists after empty output (stat err: %v)", err)
	}
}
