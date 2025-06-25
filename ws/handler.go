package ws

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	pongWait   = 10 * time.Second
	pingPeriod = (pongWait * 9) / 10 // send ping slightly before timeout
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func ServeWebSocket(hub *Hub, w http.ResponseWriter, r *http.Request) {
	roomID := r.URL.Query().Get("room")
	username := r.URL.Query().Get("username")

	if roomID == "" {
		http.Error(w, "Missing room ID", http.StatusBadRequest)
		return
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	room := hub.GetOrCreateRoom(roomID)
	client := &Client{
		Conn:     conn,
		Send:     make(chan []byte),
		Room:     room,
		Username: username,
	}
	room.Register <- client
	go client.read()
	go client.write()
}
