package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/distributed-programming-2026/gateway/internal/gateway/config"
	"github.com/urfave/cli/v3"
)

func main() {
	if err := run(); err != nil {
		slog.Error("gateway stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return command().Run(ctx, os.Args)
}

func command() *cli.Command {
	return &cli.Command{
		Name:           "gateway",
		Usage:          "Go gateway with external interceptor processing",
		DefaultCommand: "service",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Value: "config", Usage: "YAML file or directory", Sources: cli.EnvVars("GATEWAY_CONFIG")},
			&cli.BoolFlag{Name: "check-config", Usage: "validate configuration and exit"},
		},
		Commands: []*cli.Command{
			{Name: "service", Usage: "start proxy and health listeners", Action: func(ctx context.Context, c *cli.Command) error {
				conf, err := config.Load(c.String("config"))
				if err != nil {
					return err
				}
				if c.Bool("check-config") {
					_, err = fmt.Fprintln(c.Writer, "configuration valid")
					return err
				}
				return serve(ctx, conf)
			}},
			{Name: "check-config", Usage: "validate configuration and exit", Action: func(_ context.Context, c *cli.Command) error {
				if _, err := config.Load(c.String("config")); err != nil {
					return err
				}
				_, err := fmt.Fprintln(c.Writer, "configuration valid")
				return err
			}},
		},
	}
}
