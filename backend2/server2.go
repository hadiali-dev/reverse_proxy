package main

import (
	"fmt"
	"net/http"
)

	func main() {

		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "hello from port 9002")
		})
		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "everything is fine from port 9002")
		})
		http.ListenAndServe(":9002", nil)
	}
