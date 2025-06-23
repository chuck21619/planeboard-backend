package main

import (
	"log"
	"net/http"

	"github.com/chuck21619/planeboard-backend/ws"
)

func main() {
	hub := ws.NewHub()

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ws.ServeWebSocket(hub, w, r)
	})

	log.Println("Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
