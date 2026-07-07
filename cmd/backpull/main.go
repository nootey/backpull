package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"go.uber.org/zap"

	"backpull/internal/config"
	"backpull/internal/job"
	"backpull/internal/sshclient"
	"backpull/internal/store"
	"backpull/internal/util"
	logging "backpull/pkg/logger"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	only := flag.String("only", "", "comma-separated job names to run (default: all)")
	dryRun := flag.Bool("dry-run", false, "print what would run without connecting")
	flag.Parse()

	if err := run(*cfgPath, *only, *dryRun); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(cfgPath, only string, dryRun bool) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	jobs, err := util.FilterJobs(cfg.Jobs, only)
	if err != nil {
		return err
	}

	now := time.Now()
	for i := range jobs {
		jobs[i].Output = util.ExpandOutput(jobs[i].Output, now)
	}

	if dryRun {
		out := store.NewRun(cfg.Destination, now)
		fmt.Printf("dry run: %d job(s) would run on %s@%s:%d\n", len(jobs), cfg.SSH.User, cfg.SSH.Host, cfg.SSH.Port)
		for _, j := range jobs {
			path := filepath.Join(out.Dir(), j.Name, j.Output)
			if j.OutputDir != "" {
				path = filepath.Join(j.OutputDir, j.Output)
			}
			fmt.Printf("  %s\n    command: %s\n    output:  %s\n", j.Name, j.Command, path)
		}
		return nil
	}

	logger, err := logging.InitLogger(cfg.Log.Level)
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer func() { _ = logger.Sync() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	logger.Info("connecting", zap.String("host", cfg.SSH.Host), zap.String("user", cfg.SSH.User), zap.Int("port", cfg.SSH.Port))
	client, err := sshclient.Connect(logger, cfg.SSH)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	logger.Info("connected")

	out := store.NewRun(cfg.Destination, now)
	// jobs with output_dir bypass the run dir; logging it then is misleading
	dirField := zap.Skip()
	for _, j := range jobs {
		if j.OutputDir == "" {
			dirField = zap.String("dir", out.Dir())
			break
		}
	}
	logger.Info("starting run", dirField, zap.Int("jobs", len(jobs)))

	type result struct {
		name string
		err  error
	}

	start := time.Now()
	var results []result
	for _, j := range jobs {
		err := job.Run(ctx, logger, client, out, j)
		if err != nil {
			logger.Error("job failed", zap.String("job", j.Name), zap.Error(err))
		}
		results = append(results, result{name: j.Name, err: err})
		if err != nil && ctx.Err() != nil {
			break
		}
	}

	failed := 0
	for _, r := range results {
		if r.err != nil {
			failed++
			logger.Error("summary: failed", zap.String("job", r.name), zap.Error(r.err))
		} else {
			logger.Info("summary: ok", zap.String("job", r.name))
		}
	}

	logger.Info("run complete",
		zap.Int("jobs", len(results)),
		zap.Int("failed", failed),
		dirField,
		zap.Duration("duration", time.Since(start).Round(time.Millisecond)),
	)
	if failed > 0 {
		return fmt.Errorf("%d of %d jobs failed", failed, len(results))
	}
	return nil
}
