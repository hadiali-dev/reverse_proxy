package proxy

import (
	"io"
	"net"
	"net/http"
	"time"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

func sendRequest(backend *Backend, r *http.Request) (*http.Response, error) {
	req, err := http.NewRequest(r.Method, backend.URL+r.URL.RequestURI(), r.Body)
	if err != nil {
		return nil, err
	}
	req.Header = r.Header.Clone()

	req.Header.Del("Keep-Alive")
	req.Header.Del("Connection")
	req.Header.Del("Transfer-Encoding")
	req.Header.Del("Upgrade")
	req.Header.Del("Proxy-Connection")
	req.Header.Del("TE")
	req.Header.Del("Trailer")
	req.Header.Del("Proxy-Authenticate")
	req.Header.Del("Proxy-Authorization")

	// 1. Get the immediate sender IP (strip the port)
	clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)

	// 2. Check if the header already has data
	existingXFF := r.Header.Get("X-Forwarded-For")
	if existingXFF != "" {
		req.Header.Set("X-Forwarded-For", existingXFF+", "+clientIP)
	} else {
		req.Header.Set("X-Forwarded-For", clientIP)
	}
	if r.TLS != nil {
		req.Header.Set("X-Forwarded-Proto", "https")
	} else {
		req.Header.Set("X-Forwarded-Proto", "http")
	}

	req.Host = backend.URL
	return httpClient.Do(req)
}

func (p *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	backend := p.Pool.NextBackend()
	if backend == nil {
		http.Error(w, "Service Unavailable: No server alive", http.StatusServiceUnavailable)
		return
	}
	backend.AddLiveConnections()
	defer backend.DelLiveConnection()

	res, err := sendRequest(backend, r)
	if err != nil {
		backend.RecordFailure()

		if r.Method == "GET" || r.Method == "HEAD" {
			retryBackend := p.Pool.NextBackendExcluding(backend)
			if retryBackend != nil {
				retryBackend.AddLiveConnections()
				defer retryBackend.DelLiveConnection()

				res, err = sendRequest(retryBackend, r)
				backend = retryBackend
			}
		}

		if err != nil {
			backend.RecordFailure()
			http.Error(w, "bad gateway", http.StatusBadGateway)
			return
		}
	}
	defer res.Body.Close()

	backend.RecordSuccess()

	//Delete Headers from the backend response that should not be sent to the client
	res.Header.Del("Keep-Alive")
	res.Header.Del("Connection")
	res.Header.Del("Transfer-Encoding")
	res.Header.Del("Upgrade")
	res.Header.Del("Proxy-Connection")
	res.Header.Del("TE")
	res.Header.Del("Trailer")
	for key, vals := range res.Header {
		for _, val := range vals {
			w.Header().Add(key, val)
		}
	}
	w.WriteHeader(res.StatusCode)
	_, err = io.Copy(w, res.Body)
	if err != nil {
		http.Error(w, "Failed to read response", http.StatusInternalServerError)
		return
	}
}