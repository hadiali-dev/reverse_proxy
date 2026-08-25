package proxy

import (
	"sync"
	"sync/atomic"
)
type ProxyHandler struct {
	Pool	  *Pool
}
type Backend struct{
	URL string
	Alive bool
mu sync.RWMutex
LiveConnections int64
}
type Pool struct{
	Backends []*Backend
	
	Strategy Strategy
}
func (b *Backend) SetAlive(alive bool) {
    b.mu.Lock()
    defer b.mu.Unlock()
    b.Alive = alive
}
func (b *Backend)AddLiveConnections(){
atomic.AddInt64(&b.LiveConnections,1)
}
func (b *Backend) DelLiveConnection(){
atomic.AddInt64(&b.LiveConnections,-1)
}

func (b *Backend) IsAlive() bool {
    b.mu.RLock()
    defer b.mu.RUnlock()
    return b.Alive
}
func (p *Pool) NextBackend() *Backend {
var livebackend[]*Backend
 for _ ,val :=range p.Backends{
if val.IsAlive(){
livebackend=append(livebackend,val)
}

} 
if len(livebackend) == 0 {
		return nil 
	}
    return p.Strategy.Pick(livebackend)
}
type Strategy interface {
    Pick(backends []*Backend) *Backend
}
type LeastConnections struct{}

func (lc LeastConnections) Pick(backends []*Backend) *Backend {
if backends==nil{
return nil
}
var bestbackend *Backend
bestbackend=backends[0]

for i:=range backends{
if bestbackend.LiveConnections>backends[i].LiveConnections{
bestbackend=backends[i]
}
}
return bestbackend
}
type RoundRobin struct {
    counter uint64
}

func (rr *RoundRobin) Pick(backends []*Backend) *Backend {
    n := atomic.AddUint64(&rr.counter, 1)
    idx := n % uint64(len(backends))
    return backends[idx]
}