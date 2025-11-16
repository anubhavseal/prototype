package main

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func getConnection() *sql.DB {
	dsn := "root:@tcp(localhost:3306)/test?parseTime=true"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		panic(err)
	}
	// db.SetMaxOpenConns(1) // Just allow one connection
	return db
}

func benchmarkNonPooledDB(count int) {
	var wg sync.WaitGroup
	wg.Add(count)
	db := getConnection()
	startTime := time.Now()
	for i := 0; i < count; i++ {
		// go func(id int) {
		defer wg.Done()
		var err error
		// var conn *sql.Conn
		if _, err = db.Conn(context.Background()); err != nil {
			fmt.Println(i, ":db exec error:", err)
			return
		}
		fmt.Println(i, "Completed connection setup")
		time.Sleep(1 * time.Millisecond) // Simulate some work with the connection
		// conn.Close()
		// }(i)
	}
	// wg.Wait()
	duration := time.Since(startTime)
	fmt.Printf("Total time taken for %d connections: %v\n", count, duration)
}

func main() {
	// benchmarkNonPooledDB(500)
	benchmarkPooledDB(500)
}

type ConnectionPool struct {
	connections chan *sql.Conn
}

func NewConnectionPool(size int, db *sql.DB) (*ConnectionPool, error) {
	cp := &ConnectionPool{
		connections: make(chan *sql.Conn, size),
	}
	for i := 0; i < size; i++ {
		conn, err := db.Conn(context.Background())
		if err != nil {
		}
		cp.connections <- conn
	}
	return cp, nil
}

func (cp *ConnectionPool) GetConnection() (*sql.Conn, error) {
	conn := <-cp.connections
	return conn, nil
}

func (cp *ConnectionPool) SetConnection(conn *sql.Conn) {
	cp.connections <- conn
}

func benchmarkPooledDB(count int) {
	db := getConnection()
	cp, _ := NewConnectionPool(5, db)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			var msg string
			msg = fmt.Sprintf("GoRoutine %d: Waiting for connection", id)
			fmt.Println(msg)
			conn, _ := cp.GetConnection()
			msg = fmt.Sprintf("GoRoutine %d: Connection established", id)
			fmt.Println(msg)
			time.Sleep(5 * time.Second) // Siulate some work with the connection
			msg = fmt.Sprintf("GoRoutine %d: Releasing connection", id)
			fmt.Println(msg)
			cp.SetConnection(conn)
		}(i)
	}
	wg.Wait()
}
