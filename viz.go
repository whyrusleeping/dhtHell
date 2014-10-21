package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/jbenet/go-ipfs/diagnostics"
)

// Runs the visualization server to view d3 graph of the network
func RunServer(s string) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})
	http.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		diag, err := nodes[0].Diagnostics.GetDiagnostic(time.Second * 10)
		if err != nil {
			fmt.Println(err)
		}
		w.Write(diagnostics.GetGraphJson(diag))
	})
	err := http.ListenAndServe(s, nil)
	if err != nil {
		fmt.Println(err)
	}
}
