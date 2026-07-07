package util

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"backpull/internal/config"
)

// ExpandOutput replaces the {date} placeholder in an output filename with
// now formatted as 2006-01-02.
func ExpandOutput(output string, now time.Time) string {
	return strings.ReplaceAll(output, "{date}", now.Format("2006-01-02"))
}

func FilterJobs(jobs []config.Job, only string) ([]config.Job, error) {
	if only == "" {
		return jobs, nil
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
