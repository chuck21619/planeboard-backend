package main

import (
	"log"
	"net/http"
    "os"

	"github.com/chuck21619/planeboard-backend/ws"
    "github.com/joho/godotenv"
)

func main() {
    if err := godotenv.Load(); err != nil {
        log.Fatal("Error loading .env file")
    }
	hub := ws.NewHub()

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ws.ServeWebSocket(hub, w, r)
	})
    port := os.Getenv("PORT")
	
    log.Printf("Server started on :%s", port)
    log.Fatal(http.ListenAndServe(":" + port, nil))
}
