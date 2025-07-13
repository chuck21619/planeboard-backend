package ws

import (
	"net/http"
	"strconv"
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
	spectatorString := r.URL.Query().Get("spectator")
	deckUrl := r.URL.Query().Get("deckUrl")
	spectator, _ := strconv.ParseBool(spectatorString)

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
		Conn:      conn,
		Send:      make(chan []byte, 16),
		Room:      room,
		Username:  username,
		Spectator: spectator,
		DeckUrl:   deckUrl,
	}
	if room.Turn == "" {
		room.Turn = username
	}
	room.Register <- client
	go client.read()
	go client.write()
}
