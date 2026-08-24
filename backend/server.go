package main

import (
	"fmt"
	"net/http"
)

	func main() {

		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "Hello from port 9001")
		})
http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, "everything is fine frm port 9001")
		})
		http.ListenAndServe(":9001", nil)
	}
