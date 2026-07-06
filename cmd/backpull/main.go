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
	logging "backpull/pkg/logger"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	if err := run(*cfgPath); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(cfgPath string) error {
	cfg, err := config.Load(cfgPath)
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
	logger.Info("starting run", zap.String("dir", out.Dir()), zap.Int("jobs", len(cfg.Jobs)))

	start := time.Now()
	for _, j := range cfg.Jobs {
		if err := job.Run(ctx, logger, client, out, j); err != nil {
			return fmt.Errorf("job %q: %w", j.Name, err)
		}
	}

	logger.Info("run complete",
		zap.Int("jobs", len(cfg.Jobs)),
		zap.String("dir", out.Dir()),
		zap.Duration("duration", time.Since(start).Round(time.Millisecond)),
	)
	return nil
}
