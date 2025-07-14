package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"time"

	"github.com/chuck21619/planeboard-backend/ws"
	"github.com/joho/godotenv"
)

func main() {
	if os.Getenv("ENVIRONMENT") != "production" {
		if err := godotenv.Load(); err != nil {
			log.Fatal("Error loading .env file")
		}
	}
	hub := ws.NewHub()
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for {
			<-ticker.C

			hub.Mu.Lock()
			roomCount := len(hub.Rooms)
			hub.Mu.Unlock()

			logRoomCountToCSV("metrics.csv", roomCount)
		}
	}()

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

func logRoomCountToCSV(filename string, roomCount int) {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Failed to open CSV: %v", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	timestamp := time.Now().Format(time.RFC3339)
	record := []string{timestamp, fmt.Sprintf("%d", roomCount)}

	if err := writer.Write(record); err != nil {
		log.Printf("Failed to write to CSV: %v", err)
	} else {
		log.Println("Logged room count to CSV.")
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
	message := []byte("Subject: " + "\r\n" + body)

	err := smtp.SendMail("smtp.gmail.com:587", auth, address, []string{address}, message)
	if err != nil {
		return fmt.Errorf("failed to send email: %v", err)
	}
	return nil
}
