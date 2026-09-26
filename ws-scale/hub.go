package main

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// Hub maintains the set of websocket clients connected to this process
// and fans inbound Redis messages out to them.
type Hub struct {
	clients map[*Client]bool

	// broadcast is messages that already went through Redis Pub/Sub.
	// Do not also fan out in publish(): this process is a subscriber too,
	// so a local send + Redis echo would deliver the same message twice.
	broadcast chan []byte

	register   chan *Client
	unregister chan *Client

	rdb     *redis.Client
	channel string
}

func newHub(rdb *redis.Client, channel string) *Hub {
	return &Hub{
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
		rdb:        rdb,
		channel:    channel,
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
		}
	}
}

// publish sends a locally received websocket message to Redis only.
// Local clients get it when Redis delivers back onto h.broadcast.
func (h *Hub) publish(message []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := h.rdb.Publish(ctx, h.channel, message).Err(); err != nil {
		log.Println("redis publish:", err)
	}
}
