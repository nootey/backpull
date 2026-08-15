package util

import (
	"strings"
	"testing"
	"time"

	"backpull/internal/config"
)

func TestExpand(t *testing.T) {
	now := time.Date(2026, 7, 1, 15, 30, 12, 0, time.UTC)
	if got := Expand("{date}.sql", now); got != "2026-07-01.sql" {
		t.Errorf("Expand({date}.sql) = %q, want %q", got, "2026-07-01.sql")
	}
	if got := Expand("wealth-warden.sql", now); got != "wealth-warden.sql" {
		t.Errorf("Expand without placeholder = %q, want unchanged", got)
	}
	if got := Expand("/backups/{year}/homepage", now); got != "/backups/2026/homepage" {
		t.Errorf("Expand({year}) = %q, want %q", got, "/backups/2026/homepage")
	}
	if got := Expand("/backups/{year}/{date}.tar.gz", now); got != "/backups/2026/2026-07-01.tar.gz" {
		t.Errorf("Expand({year} and {date}) = %q, want %q", got, "/backups/2026/2026-07-01.tar.gz")
	}
}

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

func TestFilterJobsManual(t *testing.T) {
	jobs := []config.Job{
		{Name: "db"},
		{Name: "nextcloud-data", Manual: true},
	}

	t.Run("empty only skips manual jobs", func(t *testing.T) {
		got, err := FilterJobs(jobs, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].Name != "db" {
			t.Fatalf("got %v, want [db]", got)
		}
	})

	t.Run("naming a manual job explicitly still runs it", func(t *testing.T) {
		got, err := FilterJobs(jobs, "nextcloud-data")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].Name != "nextcloud-data" {
			t.Fatalf("got %v, want [nextcloud-data]", got)
		}
	})
}
