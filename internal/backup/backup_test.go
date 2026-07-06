package backup

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"backpull/internal/config"
	"backpull/internal/store"
)

var testTime = time.Date(2026, 7, 6, 15, 30, 12, 0, time.UTC)

var testService = config.Service{
	Name:      "postgres",
	Type:      config.TypeDBDump,
	Container: "wealth-warden-db",
	Command:   "pg_dump -U app appdb",
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

func TestDBDump(t *testing.T) {
	dest := t.TempDir()
	run := store.NewRun(dest, testTime)
	runner := &fakeRunner{output: []byte("-- PostgreSQL database dump")}

	if err := DBDump(context.Background(), runner, run, testService); err != nil {
		t.Fatalf("DBDump returned error: %v", err)
	}

	want := "docker exec wealth-warden-db pg_dump -U app appdb"
	if runner.gotCommand != want {
		t.Errorf("command = %q, want %q", runner.gotCommand, want)
	}

	final := filepath.Join(dest, "2026-07-06_153012", "postgres", "postgres.sql")
	data, err := os.ReadFile(final)
	if err != nil {
		t.Fatalf("reading final file: %v", err)
	}
	if string(data) != "-- PostgreSQL database dump" {
		t.Errorf("dump contents = %q, want %q", data, "-- PostgreSQL database dump")
	}
}

func TestDBDumpCommandError(t *testing.T) {
	dest := t.TempDir()
	run := store.NewRun(dest, testTime)
	runner := &fakeRunner{output: []byte("partial dump"), err: errors.New("exit status 1")}

	if err := DBDump(context.Background(), runner, run, testService); err == nil {
		t.Fatal("DBDump succeeded, want error")
	}

	dir := filepath.Join(dest, "2026-07-06_153012", "postgres")
	if _, err := os.Stat(filepath.Join(dir, "postgres.sql")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("final file exists after failed command (stat err: %v)", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "postgres.sql.partial")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf(".partial left behind after failed command (stat err: %v)", err)
	}
}

func TestDBDumpEmptyOutput(t *testing.T) {
	dest := t.TempDir()
	run := store.NewRun(dest, testTime)
	runner := &fakeRunner{output: nil}

	err := DBDump(context.Background(), runner, run, testService)
	if err == nil {
		t.Fatal("DBDump succeeded on empty output, want error")
	}

	final := filepath.Join(dest, "2026-07-06_153012", "postgres", "postgres.sql")
	if _, err := os.Stat(final); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("final file exists after empty dump (stat err: %v)", err)
	}
}
