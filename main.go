package main

import (
	"hello-go/proxy"

	"net/http"
)

func main() {
	http.ListenAndServe(":8080", &proxy.ProxyHandler{TargetUrl: "http://localhost:9001"})
}
