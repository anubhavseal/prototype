package main

import (
	"database/sql"
	"log"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func getConnection() (*sql.DB, error) {
	dsn := "root:@tcp(localhost:3306)/test?parseTime=true"
	db, err := sql.Open("mysql", dsn)
	// db.SetMaxOpenConns(1)
	if err != nil {
		log.Println("error opening db connection.", err)
		return nil, err
	}
	return db, nil
}

type ConnectionPool struct {
	connections chan *sql.DB
}

func NewConnectionPool(count int) ConnectionPool {
	cp := ConnectionPool{}
	cp.connections = make(chan *sql.DB, count)
	for i := 0; i < count; i++ {
		db, _ := getConnection()
		cp.connections <- db
	}
	return cp
}

func (cp *ConnectionPool) getConnection() *sql.DB {
	db := <-cp.connections
	return db
}

func (cp *ConnectionPool) releaseConnection(db *sql.DB) {
	cp.connections <- db
}

func benchmarkPooledDB() {
	startTime := time.Now()
	var wg sync.WaitGroup
	cp := NewConnectionPool(10)
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db := cp.getConnection()
			_, err := db.Exec("select sleep(0.01);")
			if err != nil {
				panic(err)
			}
			cp.releaseConnection(db)
		}()
	}
	wg.Wait()
	log.Println("Duration", time.Since(startTime))
}

func benchmarkNonPooledDB() {
	startTime := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			db, _ := getConnection()
			_, err := db.Exec("select sleep(0.01);")
			if err != nil {
				panic(err)
			}
			// db.Close()
		}()
	}
	wg.Wait()
	log.Println("Duration", time.Since(startTime))
}

func main() {
	// benchmarkNonPooledDB()
	// benchmarkPooledDB()

}

var semaphoreMap map[string]chan struct{}
var cache map[string]string
var mu sync.Mutex

func fetchFromDB() {
	time.Sleep(5 * time.Second)
}

func GetFromCache(key string) string {
	mu.Lock()
	if val, ok := cache[key]; ok {
		mu.Unlock()
		return val
	}

	if _, ok := semaphoreMap[key]; !ok {
		semaphoreMap[key] = make(chan struct{})
		mu.Unlock()

		fetchFromDB()

		mu.Lock()
		cache[key] = "world"
		signal := semaphoreMap[key]
		delete(semaphoreMap, key)
		close(signal)
		value := cache[key]
		mu.Unlock()

		return value
	} else {
		ch := semaphoreMap[key]
		mu.Unlock()
		<-ch
		mu.Lock()
		value := cache[key]
		mu.Unlock()
		return value
	}

}

func simulateRequests() {
	cache = map[string]string{}
	semaphoreMap = make(map[string]chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			GetFromCache("hello")
		}()
	}
	wg.Wait()
}
