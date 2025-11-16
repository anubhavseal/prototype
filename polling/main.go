package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var db *sql.DB

func getConnection() (*sql.DB, error) {
	db, err := sql.Open("mysql", "root:@tcp(localhost:3306)/test")
	if err != nil {
		return nil, err
	}
	return db, nil
}

// startDeployment simulates a deployment lifecycle for server with a fixed id (8).
// It updates statuses in the DB so the polling endpoints can observe transitions.
func startDeployment() (int, error) {
	serverID := int(time.Now().UnixNano() % 10000)
	log.Default().Println("Starting Deployment")
	_, err := db.Exec("INSERT INTO servers (status, server_id) VALUES (?, ?)", "INIT", serverID)
	if err != nil {
		log.Default().Printf("Error creating a new server %v", err)
		return 0, err
	}
	time.Sleep(3 * time.Second)
	_, err = db.Exec("UPDATE servers SET status=? WHERE server_id = ?", "CREATING", serverID)
	if err != nil {
		log.Default().Printf("Error updating server %v", err)
		return 0, err
	}
	time.Sleep(10 * time.Second)
	_, err = db.Exec("UPDATE servers SET status=? WHERE server_id = ?", "CREATED", serverID)
	if err != nil {
		log.Default().Printf("Error updating server %v", err)
		return 0, err
	}
	log.Default().Println("Completed Deployment")
	return serverID, nil
}

// shortPoll performs a single-shot check and returns the current status (or empty string on error).
func shortPoll(id int) string {
	var status string
	err := db.QueryRow("SELECT status FROM servers WHERE server_id = ?", id).Scan(&status)
	if err != nil {
		log.Default().Println("error fetching status:", err)
		return ""
	}
	// minor wait to mimic original behavior, but return immediately
	if status != "CREATED" {
		time.Sleep(1 * time.Second)
	}
	return status
}

// longPoll blocks until the status becomes CREATED and then returns that status.
func longPoll(id int, currentStatus string) string {
	var status string
	for {
		log.Default().Println("Polling")
		err := db.QueryRow("SELECT status FROM servers WHERE server_id = ?", id).Scan(&status)
		if err != nil {
			log.Default().Println("error fetching status:", err)
		}
		if status == currentStatus {
			time.Sleep(1 * time.Second)
			continue
		}
		return status
	}
}

func shortPollHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	status := shortPoll(id)
	resp := map[string]interface{}{"id": id, "status": status}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func longPollHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	currentStatus := r.URL.Query().Get("currentStatus")
	if idStr == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	status := longPoll(id, currentStatus)
	resp := map[string]interface{}{"id": id, "status": status}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func deploy(w http.ResponseWriter, r *http.Request) {
	serverID, err := startDeployment()
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		resp := map[string]interface{}{"error": err}
		json.NewEncoder(w).Encode(resp)
		return
	}
	resp := map[string]interface{}{"serverID": serverID}
	json.NewEncoder(w).Encode(resp)
}

func main() {
	var err error
	db, err = getConnection()
	if err != nil {
		panic(err)
	}

	// kick off a simulated deployment for server id=8
	// go startDeployment()

	// HTTP APIs
	http.HandleFunc("/deploy", deploy)
	http.HandleFunc("/short-poll", shortPollHandler) // GET /short-poll?id=8
	http.HandleFunc("/long-poll", longPollHandler)   // GET /long-poll?id=8

	log.Default().Println("Starting HTTP server on :8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Default().Fatal(err)
	}
}
