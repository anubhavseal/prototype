package main

import (
	"fmt"
	"sync"
	"time"
)

type cache struct {
	store map[string]string
}

func (c *cache) Get(key string) (string, bool) {
	value, exists := c.store[key]
	if exists {
		fmt.Println("Value:", value)
	} else {
		fmt.Println("Key not found")
	}
	return value, exists
}

func (c *cache) Set(key, value string) {
	c.store[key] = value
	fmt.Println("Set key:", key, "with value:", value)
}

type DB struct {
	store map[string]string
}

func (c *DB) Get(key string) (string, bool) {
	value, exists := c.store[key]
	if exists {
		fmt.Println("Value:", value)
	} else {
		fmt.Println("Key not found")
	}
	return value, exists
}

func (c *DB) Set(key, value string) {
	c.store[key] = value
	fmt.Println("Set key:", key, "with value:", value)
}

var semMap map[string]chan struct{}
var mu sync.Mutex
var c *cache
var db *DB

func main() {
	semMap = make(map[string]chan struct{})
	c = &cache{store: make(map[string]string)}
	db := &DB{store: make(map[string]string)}
	db.Set("foo", "bar")
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			value := GetFromCache("foo")
			fmt.Println("Retrieved value:", value)
		}()
	}
	wg.Wait()
}

func GetFromCache(key string) string {
	mu.Lock()

	if value, found := c.Get(key); found {
		mu.Unlock()
		return value
	}

	// value does not exist in cache
	_, exists := semMap[key]

	// first goroutine to request the key
	if !exists {
		semMap[key] = make(chan struct{})
		mu.Unlock()

		time.Sleep(100 * time.Millisecond)

		value, _ := db.Get(key)

		mu.Lock()
		c.Set(key, value)
		delete(semMap, key)
		mu.Unlock()

		close(semMap[key]) // notify other goroutines

		return value
	} else {
		mu.Unlock()
		<-semMap[key]

		mu.Lock()
		value, _ := c.Get(key)
		mu.Unlock()
		return value
	}

}
