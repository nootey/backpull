package util

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"backpull/internal/config"
)

// Expand replaces {date}, {year} and {month} placeholders in a job's output or
// output_dir with now formatted as 2006-01-02, 2006 and 01, respectively.
func Expand(s string, now time.Time) string {
	s = strings.ReplaceAll(s, "{date}", now.Format("2006-01-02"))
	s = strings.ReplaceAll(s, "{year}", now.Format("2006"))
	s = strings.ReplaceAll(s, "{month}", now.Format("01"))
	return s
}

func FilterJobs(jobs []config.Job, only string) ([]config.Job, error) {
	if only == "" {
		var selected []config.Job
		for _, j := range jobs {
			if !j.Manual {
				selected = append(selected, j)
			}
		}
		return selected, nil
	}

	wanted := make(map[string]bool)
	for _, name := range strings.Split(only, ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			wanted[name] = true
		}
	}
	if len(wanted) == 0 {
		return nil, fmt.Errorf("-only %q selects no jobs", only)
	}

	var selected []config.Job
	for _, j := range jobs {
		if wanted[j.Name] {
			selected = append(selected, j)
			delete(wanted, j.Name)
		}
	}
	if len(wanted) > 0 {
		unknown := make([]string, 0, len(wanted))
		for name := range wanted {
			unknown = append(unknown, name)
		}
		sort.Strings(unknown)
		names := make([]string, len(jobs))
		for i, j := range jobs {
			names[i] = j.Name
		}
		return nil, fmt.Errorf("unknown job(s) %s; available: %s",
			strings.Join(unknown, ", "), strings.Join(names, ", "))
	}
	return selected, nil
}
