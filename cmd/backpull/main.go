package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
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
	flag.Parse()

	if err := run(*cfgPath, *only); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(cfgPath, only string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	jobs, err := util.FilterJobs(cfg.Jobs, only)
	if err != nil {
		return err
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

	out := store.NewRun(cfg.Destination, time.Now())
	logger.Info("starting run", zap.String("dir", out.Dir()), zap.Int("jobs", len(jobs)))

	start := time.Now()
	for _, j := range jobs {
		if err := job.Run(ctx, logger, client, out, j); err != nil {
			return fmt.Errorf("job %q: %w", j.Name, err)
		}
	}

	logger.Info("run complete",
		zap.Int("jobs", len(jobs)),
		zap.String("dir", out.Dir()),
		zap.Duration("duration", time.Since(start).Round(time.Millisecond)),
	)
	return nil
}
