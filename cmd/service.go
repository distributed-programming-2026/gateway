package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"os"

	"github.com/distributed-programming-2026/gateway/internal/gateway/config"
	"github.com/distributed-programming-2026/gateway/internal/gateway/infra/server"
)

func serve(ctx context.Context, c *config.Config) (result error) {
	runtime, err := server.New(c)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, runtime.Close()) }()
	listener, err := net.Listen("tcp", envDefault("GATEWAY_HTTP_ADDRESS", config.Address(c.Gateway.Port)))
	if err != nil {
		return err
	}
	defer func() { _ = listener.Close() }()
	health, err := net.Listen("tcp", envDefault("GATEWAY_HEALTH_ADDRESS", config.Address(c.Gateway.HealthzPort)))
	if err != nil {
		return err
	}
	defer func() { _ = health.Close() }()
	healthListeners := []net.Listener{health}
	if c.Gateway.GatewayHealthHost != 0 && c.Gateway.GatewayHealthHost != c.Gateway.HealthzPort {
		l, err := net.Listen("tcp", config.Address(c.Gateway.GatewayHealthHost))
		if err != nil {
			return err
		}
		defer func() { _ = l.Close() }()
		healthListeners = append(healthListeners, l)
	}
	slog.Info("gateway listening", "proxy", listener.Addr(), "health", health.Addr())
	return runtime.Serve(ctx, listener, healthListeners...)
}

func envDefault(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
