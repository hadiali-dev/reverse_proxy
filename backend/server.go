package main

import (
	"fmt"
	"net/http"
	"time"
)

	func main() {

		http.HandleFunc("/fd", func(w http.ResponseWriter, r *http.Request) {
time.Sleep(11* time.Second)

			fmt.Fprint(w, "Hello, World!")
		})
		http.ListenAndServe(":9001", nil)
	}
