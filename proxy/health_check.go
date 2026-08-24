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
func(p *Pool) StartHealthChecks(){
for _,b:=range p.Backends{
b.HealthCheck()}
Ticker:=time.NewTicker(5*time.Second)
defer Ticker.Stop()
for range Ticker.C{
for _,b:=range p.Backends{
b.HealthCheck()}
}
}