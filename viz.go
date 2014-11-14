package main

import (
	"fmt"
	"net/http"

	"github.com/jbenet/go-ipfs/diagnostics"
)

// Runs the visualization server to view d3 graph of the network
func RunServer(s string) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})
	http.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		diag, err := controllers[0].RunCommand([]string{"0", "diag"})
		if err != nil {
			fmt.Println(err)
		}
		_ = diag
		fmt.Println("NOT YET IMPLEMENTED!!!")
		var dinfo []*diagnostics.DiagInfo
		w.Write(diagnostics.GetGraphJson(dinfo))
	})
	err := http.ListenAndServe(s, nil)
	if err != nil {
		fmt.Println(err)
	}
}
