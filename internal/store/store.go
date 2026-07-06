package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

type Run struct {
	dir string
}

func NewRun(destination string, now time.Time) *Run {
	return &Run{dir: filepath.Join(destination, now.Format("2006-01-02_150405"))}
}

func (r *Run) Dir() string {
	return r.dir
}

func (r *Run) Create(job, filename string) (*File, error) {
	dir := filepath.Join(r.dir, job)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating %s: %w", dir, err)
	}

	final := filepath.Join(dir, filename)
	if _, err := os.Stat(final); err == nil {
		return nil, fmt.Errorf("%s already exists", final)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("checking %s: %w", final, err)
	}

	f, err := os.OpenFile(final+".partial", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, fmt.Errorf("creating %s.partial: %w", final, err)
	}
	return &File{f: f, final: final}, nil
}

// File writes to <final>.partial until Commit renames it into place, so an
// interrupted transfer never leaves a partial file that looks like a backup.
type File struct {
	f         *os.File
	final     string
	committed bool
}

func (f *File) Write(p []byte) (int, error) {
	return f.f.Write(p)
}

// Path is the final destination the file will land at after Commit.
func (f *File) Path() string {
	return f.final
}

func (f *File) Commit() error {
	if err := f.f.Sync(); err != nil {
		return fmt.Errorf("syncing %s: %w", f.f.Name(), err)
	}
	if err := f.f.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", f.f.Name(), err)
	}
	if err := os.Rename(f.f.Name(), f.final); err != nil {
		return fmt.Errorf("finalizing %s: %w", f.final, err)
	}
	f.committed = true
	return nil
}

// Close is a no-op after Commit; otherwise it discards the partial file.
func (f *File) Close() error {
	if f.committed {
		return nil
	}
	_ = f.f.Close()
	if err := os.Remove(f.f.Name()); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("removing %s: %w", f.f.Name(), err)
	}
	return nil
}
