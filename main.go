package main

import (
	"hello-go/proxy"
	"net/http"
)
func main() {
    pool := &proxy.Pool{
        Backends: []*proxy.Backend{
            {URL: "http://localhost:9001"},
            {URL: "http://localhost:9002"},
        },
    }

    go pool.StartHealthChecks()

    http.ListenAndServe(":8080", &proxy.ProxyHandler{Pool: pool})
}
