package store

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var testTime = time.Date(2026, 7, 6, 15, 30, 12, 0, time.UTC)

func TestCommit(t *testing.T) {
	dest := t.TempDir()
	run := NewRun(dest, testTime)

	f, err := run.Create("jellyfin", "jellyfin.sql.gz")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := f.Write([]byte("dump contents")); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if err := f.Commit(); err != nil {
		t.Fatalf("Commit returned error: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Errorf("Close after Commit returned error: %v", err)
	}

	final := filepath.Join(dest, "2026-07-06_153012", "jellyfin", "jellyfin.sql.gz")
	data, err := os.ReadFile(final)
	if err != nil {
		t.Fatalf("reading final file: %v", err)
	}
	if string(data) != "dump contents" {
		t.Errorf("final file contents = %q, want %q", data, "dump contents")
	}
	if _, err := os.Stat(final + ".partial"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf(".partial still exists after Commit (stat err: %v)", err)
	}
}

func TestPartialBeforeCommit(t *testing.T) {
	dest := t.TempDir()
	run := NewRun(dest, testTime)

	f, err := run.Create("jellyfin", "jellyfin.sql.gz")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	defer func() { _ = f.Close() }()

	final := filepath.Join(dest, "2026-07-06_153012", "jellyfin", "jellyfin.sql.gz")
	if _, err := os.Stat(final); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("final file exists before Commit (stat err: %v)", err)
	}
	if _, err := os.Stat(final + ".partial"); err != nil {
		t.Errorf("expected .partial during write: %v", err)
	}
}

func TestCloseWithoutCommit(t *testing.T) {
	dest := t.TempDir()
	run := NewRun(dest, testTime)

	f, err := run.Create("jellyfin", "jellyfin.sql.gz")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if _, err := f.Write([]byte("interrupted")); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	final := filepath.Join(dest, "2026-07-06_153012", "jellyfin", "jellyfin.sql.gz")
	if _, err := os.Stat(final); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("final file exists after abort (stat err: %v)", err)
	}
	if _, err := os.Stat(final + ".partial"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf(".partial left behind after abort (stat err: %v)", err)
	}
}

func TestCreateExistingFinal(t *testing.T) {
	dest := t.TempDir()
	run := NewRun(dest, testTime)

	dir := filepath.Join(dest, "2026-07-06_153012", "jellyfin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "jellyfin.sql.gz"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := run.Create("jellyfin", "jellyfin.sql.gz"); err == nil {
		t.Fatal("Create succeeded over existing file, want error")
	}
}

func TestCreateMissingDestination(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "unmounted")
	run := NewRun(dest, testTime)

	if _, err := run.Create("jellyfin", "jellyfin.sql.gz"); err == nil {
		t.Fatal("Create succeeded on missing destination, want error")
	}
}

func TestCreateInMissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "unmounted")
	if _, err := CreateIn(dir, "wealth-warden.sql"); err == nil {
		t.Fatal("CreateIn succeeded on missing dir, want error")
	}
}

func TestCreateInUnderCreatesMissingDirs(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "notes", "2027")

	f, err := CreateInUnder(root, dir, "2027-01-01.tar.gz")
	if err != nil {
		t.Fatalf("CreateInUnder returned error: %v", err)
	}
	if _, err := f.Write([]byte("archive")); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if err := f.Commit(); err != nil {
		t.Fatalf("Commit returned error: %v", err)
	}
	_ = f.Close()

	data, err := os.ReadFile(filepath.Join(dir, "2027-01-01.tar.gz"))
	if err != nil {
		t.Fatalf("reading final file: %v", err)
	}
	if string(data) != "archive" {
		t.Errorf("contents = %q, want %q", data, "archive")
	}
}

// the root standing in for a mounted drive must still be checked
func TestCreateInUnderMissingRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "unmounted")
	dir := filepath.Join(root, "notes", "2027")

	if _, err := CreateInUnder(root, dir, "2027-01-01.tar.gz"); err == nil {
		t.Fatal("CreateInUnder succeeded on missing root, want error")
	}
	if _, err := os.Stat(root); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("root was created despite being missing (stat err: %v)", err)
	}
}

func TestCreateInUnderRootIsFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "notafile")
	if err := os.WriteFile(root, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := CreateInUnder(root, filepath.Join(root, "notes"), "x.tar.gz"); err == nil {
		t.Fatal("CreateInUnder succeeded with a file as root, want error")
	}
}

func TestCreateInOverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "2026-07-01.sql"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	f, err := CreateIn(dir, "2026-07-01.sql")
	if err != nil {
		t.Fatalf("CreateIn returned error: %v", err)
	}
	if _, err := f.Write([]byte("new dump")); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if err := f.Commit(); err != nil {
		t.Fatalf("Commit returned error: %v", err)
	}
	_ = f.Close()

	data, err := os.ReadFile(filepath.Join(dir, "2026-07-01.sql"))
	if err != nil {
		t.Fatalf("reading final file: %v", err)
	}
	if string(data) != "new dump" {
		t.Errorf("final file contents = %q, want %q", data, "new dump")
	}
}

func TestTwoServicesShareRun(t *testing.T) {
	dest := t.TempDir()
	run := NewRun(dest, testTime)

	for _, svc := range []string{"jellyfin", "nextcloud"} {
		f, err := run.Create(svc, svc+".tar.gz")
		if err != nil {
			t.Fatalf("Create(%s) returned error: %v", svc, err)
		}
		if err := f.Commit(); err != nil {
			t.Fatalf("Commit(%s) returned error: %v", svc, err)
		}
		_ = f.Close()
	}

	runDir := filepath.Join(dest, "2026-07-06_153012")
	for _, svc := range []string{"jellyfin", "nextcloud"} {
		if _, err := os.Stat(filepath.Join(runDir, svc, svc+".tar.gz")); err != nil {
			t.Errorf("missing %s archive in run dir: %v", svc, err)
		}
	}
}
