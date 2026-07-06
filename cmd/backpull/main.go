package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"backpull/internal/backup"
	"backpull/internal/config"
	"backpull/internal/sshclient"
	"backpull/internal/store"
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	client, err := sshclient.Connect(cfg.SSH)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()
	fmt.Fprintf(os.Stderr, "connected to %s@%s\n", cfg.SSH.User, cfg.SSH.Host)

	backups := store.NewRun(cfg.Destination, time.Now())
	for _, svc := range cfg.Services {
		switch svc.Type {
		case config.TypeDBDump:
			fmt.Fprintf(os.Stderr, "backing up %s...\n", svc.Name)
			if err := backup.DBDump(ctx, client, backups, svc); err != nil {
				return fmt.Errorf("backing up %s: %w", svc.Name, err)
			}
		default:
			fmt.Fprintf(os.Stderr, "skipping %s: type %s not implemented yet\n", svc.Name, svc.Type)
		}
	}
	return nil
}
