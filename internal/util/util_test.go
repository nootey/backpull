package util

import (
	"strings"
	"testing"

	"backpull/internal/config"
)

func TestFilterJobs(t *testing.T) {
	jobs := []config.Job{
		{Name: "db"},
		{Name: "grafana"},
		{Name: "configs"},
	}

	t.Run("empty selects all", func(t *testing.T) {
		got, err := FilterJobs(jobs, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("got %d jobs, want 3", len(got))
		}
	})

	t.Run("single name", func(t *testing.T) {
		got, err := FilterJobs(jobs, "grafana")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].Name != "grafana" {
			t.Fatalf("got %v, want [grafana]", got)
		}
	})

	t.Run("preserves config order", func(t *testing.T) {
		got, err := FilterJobs(jobs, "configs, db")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 || got[0].Name != "db" || got[1].Name != "configs" {
			t.Fatalf("got %v, want [db configs]", got)
		}
	})

	t.Run("unknown name errors with available list", func(t *testing.T) {
		_, err := FilterJobs(jobs, "db,nope")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "nope") {
			t.Fatalf("error %q does not name the unknown job", err)
		}
		if !strings.Contains(err.Error(), "db, grafana, configs") {
			t.Fatalf("error %q does not list available jobs", err)
		}
	})

	t.Run("only separators errors", func(t *testing.T) {
		if _, err := FilterJobs(jobs, " , "); err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
