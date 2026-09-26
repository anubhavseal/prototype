package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	addr      = flag.String("addr", ":8080", "http service address")
	redisAddr = flag.String("redis", "localhost:6379", "redis address")
	channel   = flag.String("channel", "chat", "redis pub/sub channel shared by all servers")
)

func serveHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.ServeFile(w, r, "home.html")
}

func main() {
	flag.Parse()

	ctx := context.Background()
	rdb := redis.NewClient(&redis.Options{Addr: *redisAddr})
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis ping: %v", err)
	}

	hub := newHub(rdb, *channel)
	go hub.run()

	// One process-wide subscription. Redis is the star center: every server
	// leaf publishes here and fans out only the messages it receives.
	pubsub := rdb.Subscribe(ctx, *channel)
	defer pubsub.Close()

	go func() {
		for msg := range pubsub.Channel() {
			hub.broadcast <- []byte(msg.Payload)
		}
	}()

	http.HandleFunc("/", serveHome)
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		log.Println("/ws", r.RemoteAddr)
		serveWs(hub, w, r)
	})

	server := &http.Server{
		Addr:              *addr,
		ReadHeaderTimeout: 3 * time.Second,
	}
	log.Printf("listening on %s (redis %s channel %q)", *addr, *redisAddr, *channel)
	if err := server.ListenAndServe(); err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
