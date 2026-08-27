package proxy

import (
	"encoding/json"
	"net/http"
)

type BackendStatus struct {
	URL             string `json:"url"`
	Alive           bool   `json:"alive"`
	State           string `json:"state"`
	LiveConnections int64  `json:"live_connections"`
}

type StatusHandler struct {
	Pool *Pool
}

func (s *StatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var statuses []BackendStatus
	for _, b := range s.Pool.Backends {
		statuses = append(statuses, BackendStatus{
			URL:             b.URL,
			Alive:           b.IsAlive(),
			State:           b.State(),
			LiveConnections: b.LiveConnections,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(statuses)
}