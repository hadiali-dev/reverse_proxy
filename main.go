package main

import (
	"context"
	"hello-go/proxy"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg, err := proxy.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	var backends []*proxy.Backend
	for _, bc := range cfg.Backends {
		backends = append(backends, &proxy.Backend{URL: bc.URL})
	}

	var strategy proxy.Strategy
	switch cfg.Strategy {
	case "round_robin":
		strategy = &proxy.RoundRobin{}
	case "least_connections":
		strategy = proxy.LeastConnections{}
	default:
		log.Fatalf("invalid strategy %q in config, expected 'round_robin' or 'least_connections'", cfg.Strategy)
	}

	pool := &proxy.Pool{
		Backends: backends,
		Strategy: strategy,
	}

	go pool.StartHealthChecks(time.Duration(cfg.HealthCheckIntervalSeconds) * time.Second)

	mux := http.NewServeMux()
	mux.Handle("/status", &proxy.StatusHandler{Pool: pool})
	mux.Handle("/", &proxy.ProxyHandler{Pool: pool})

	srv := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: mux,
	}

	// Run the server in its own goroutine so main() is free to wait for a shutdown signal.
	go func() {
		slog.Info("proxy listening", "addr", cfg.ListenAddr)
		if err := srv.ListenAndServeTLS("cert.pem", "key.pem"); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Block here until the OS sends SIGINT (Ctrl+C) or SIGTERM (what Kubernetes/systemd send on shutdown).
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	slog.Info("shutdown signal received, draining connections")

	// Give in-flight requests up to 10 seconds to finish before forcing exit.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("graceful shutdown failed", "error", err)
	} else {
		slog.Info("shutdown complete")
	}
}	