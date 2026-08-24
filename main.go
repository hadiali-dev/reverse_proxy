package main

import (
	"hello-go/proxy"

	"net/http"
)

func main() {
	http.ListenAndServe(":8080", &proxy.ProxyHandler{
	Pool: &proxy.Pool{
	Backends: []*proxy.Backend{
		{URL: "http://localhost:9001"},
		{URL: "http://localhost:9002"},
	},
	},
})
}
