package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"webhookrelay/internal/config"
	"webhookrelay/internal/relay"
	"webhookrelay/internal/server"
)

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "", "Path to JSON config file (or set WEBHOOKRELAY_CONFIG)")
	flag.Parse()

	if configPath == "" {
		configPath = os.Getenv("WEBHOOKRELAY_CONFIG")
	}
	if configPath == "" {
		_, _ = fmt.Fprintln(os.Stderr, "missing config: pass -config or set WEBHOOKRELAY_CONFIG")
		os.Exit(2)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	cfg, err := config.Load(configPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	resolved, err := config.ResolveRelays(cfg)
	if err != nil {
		logger.Error("failed to resolve relays", "error", err)
		os.Exit(1)
	}

	fwd := relay.NewForwarder(relay.ForwarderConfig{
		Logger:         logger,
		Concurrency:    cfg.Server.Concurrency,
		ForwardTimeout: cfg.Server.ForwardTimeout(),
	})

	srv := server.New(server.Config{
		Logger:     logger,
		ListenAddr: cfg.Server.ListenAddr,
		Relays:     resolved,
		Forwarder:  fwd,
	})

	logger.Info("starting server", "listen_addr", cfg.Server.ListenAddr, "relay_count", len(resolved))
	for _, r := range resolved {
		logger.Info("relay", "name", r.Name, "id", r.ID, "path", r.ListenPath, "methods", r.Methods, "destinations", len(r.Destinations))
	}

	if err := srv.Run(); err != nil {
		logger.Error("server exited", "error", err)
		os.Exit(1)
	}
}
