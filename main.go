package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"time"

	"github.com/chuck21619/planeboard-backend/ws"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

func main() {
	if os.Getenv("ENVIRONMENT") != "production" {
		if err := godotenv.Load(); err != nil {
			log.Fatal("Error loading .env file")
		}
	}

	hub := ws.NewHub()

	//metrics
	if os.Getenv("ENVIRONMENT") == "production" {
		dbURL := os.Getenv("SUPABASE_DB_URL")
		if dbURL == "" {
			log.Println("WARNING: SUPABASE_DB_URL is not set")
		}
		db, err := sql.Open("postgres", dbURL)
		if err != nil {
			log.Printf("WARNING: Failed to connect to DB: %v", err)
		}
		defer db.Close()
		go func() {
			ticker := time.NewTicker(1 * time.Hour)
			defer ticker.Stop()
			for {
				<-ticker.C
				hub.Mu.Lock()
				roomCount := len(hub.Rooms)
				hub.Mu.Unlock()
				logRoomCountToSupabase(db, roomCount)
			}
		}()
	}

	http.HandleFunc("/health", withCORS(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ws.ServeWebSocket(hub, w, r)
	})

	http.HandleFunc("/report", withCORS(handleReport))

	port := os.Getenv("PORT")
	log.Printf("Server started on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "http://localhost:5173" || origin == "https://planeboard-frontend.onrender.com" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		}
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

func logRoomCountToSupabase(db *sql.DB, count int) {
	_, err := db.Exec(`INSERT INTO metrics (timestamp, room_count) VALUES ($1, $2)`, time.Now(), count)
	if err != nil {
		log.Printf("Failed to log metrics: %v", err)
	}
}

func handleReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	if payload.Message == "" {
		http.Error(w, "Empty message", http.StatusBadRequest)
		return
	}
	err := SendEmail(payload.Message)
	if err != nil {
		log.Printf("Failed to send report: %v", err)
		http.Error(w, "Failed to send report", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func SendEmail(body string) error {
	address := os.Getenv("GMAIL_ADDRESS")
	password := os.Getenv("GMAIL_APP_PASSWORD")
	auth := smtp.PlainAuth("", address, password, "smtp.gmail.com")
	subject := "Planeboard Report"
	message := []byte(
		"To: " + address + "\r\n" +
			"Subject: " + subject + "\r\n" +
			"Content-Type: text/plain; charset=\"UTF-8\"\r\n" +
			"\r\n" +
			body + "\r\n")

	err := smtp.SendMail("smtp.gmail.com:587", auth, address, []string{address}, message)
	if err != nil {
		return fmt.Errorf("failed to send email: %v", err)
	}
	return nil
}
