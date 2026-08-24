package main

import (
	"fmt"
	"net/http"
)

	func main() {

		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "Hello from port 9002")
		})
		http.ListenAndServe(":9002", nil)
	}
