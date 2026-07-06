package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"backpull/internal/config"
	"backpull/internal/sshclient"
)

func main() {
	cfgPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	if err := run(*cfgPath, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(cfgPath string, args []string) error {
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

	command := "uname -a"
	if len(args) > 0 {
		command = strings.Join(args, " ")
	}
	fmt.Fprintf(os.Stderr, "connected to %s@%s, running: %s\n", cfg.SSH.User, cfg.SSH.Host, command)
	return client.RunCommand(ctx, command, os.Stdout)
}
