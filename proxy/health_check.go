package proxy

import (
	"net/http"
	"time"
)

func (b *Backend) HealthCheck() {
    res, err := http.Get(b.URL + "/health")
    if err != nil {
        b.SetAlive(false)
        return
    }
    defer res.Body.Close()

    if res.StatusCode == http.StatusOK {
        b.SetAlive(true)
    } else {
        b.SetAlive(false)
    }
}
func (p *Pool) StartHealthChecks(interval time.Duration) {
    for _, b := range p.Backends {
        b.HealthCheck()
    }
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    for range ticker.C {
        for _, b := range p.Backends {
            b.HealthCheck()
        }
    }
}