package main

import (
	"hello-go/proxy"
	"log"
	"log/slog"
	"net/http"
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

slog.Info("proxy listening", "addr", cfg.ListenAddr)
log.Fatal(http.ListenAndServe(cfg.ListenAddr, mux))
}