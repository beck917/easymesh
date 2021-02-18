package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/beck917/easymesh/proxy"
)

func indexHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "hello world")
}

func main() {
	http.HandleFunc("/", indexHandler)
	go http.ListenAndServe(":6060", nil)
	// start the client proxy
	go proxy.Run(&proxy.ServerOptions{
		Name:     "Client Listener Proxy",
		Address:  ":8082",
		Endpoint: "http://127.0.0.1:8083",
		Protocol: "http",
	})

	time.Sleep(1 * time.Second)

	// start the server proxy
	proxy.Run(&proxy.ServerOptions{
		Name:     "Server Listener Proxy",
		Address:  ":8083",
		Endpoint: "http://127.0.0.1:6060",
		Protocol: "http",
	})
}
