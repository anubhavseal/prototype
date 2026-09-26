package main

import (
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
	//db.SetMaxOpenConns(100) // Just allow one connection
	return db
}

func benchmarkNonPooledDB(count int) {
	var wg sync.WaitGroup
	wg.Add(count)
	startTime := time.Now()
	db := getConnection()
	for i := 0; i < count; i++ {
		go func(id int) {
			defer wg.Done()
			if _, err := db.Exec("select SLEEP(0.01)"); err != nil {
				fmt.Println(i, err)
			}

			// var err error
			// var conn *sql.Conn
			// if conn, err = db.Conn(context.Background()); err != nil {
			// 	fmt.Println(i, ":db exec error:", err)
			// 	return
			// }
			// fmt.Println(i, "Completed connection setup")
			// time.Sleep(1 * time.Millisecond) // Simulate some work with the connection
			// conn.Close()
		}(i)
	}
	wg.Wait()
	db.Close()
	duration := time.Since(startTime)
	fmt.Printf("Total time taken for %d connections: %v\n", count, duration)
}

func main() {
	// benchmarkNonPooledDB(50000)
	benchmarkPooledDB(5000)
}

type ConnectionPool struct {
	connections chan int
}

func NewConnectionPool(size int, db *sql.DB) (*ConnectionPool, error) {
	cp := &ConnectionPool{
		connections: make(chan int, size),
	}
	for i := 0; i < size; i++ {
		// conn, err := db.Conn(context.Background())
		// if err != nil {
		// }
		cp.connections <- i
	}
	return cp, nil
}

func (cp *ConnectionPool) GetConnection() (int, error) {
	conn := <-cp.connections
	return conn, nil
}

func (cp *ConnectionPool) SetConnection(conn int) {
	cp.connections <- conn
}

func benchmarkPooledDB(count int) {
	startTime := time.Now()
	db := getConnection()
	cp, _ := NewConnectionPool(200, db)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cp.GetConnection()
			if _, err := db.Exec("select SLEEP(0.01)"); err != nil {
				fmt.Println(i, err)
			}
			cp.SetConnection(1)
			// var msg string
			// msg = fmt.Sprintf("GoRoutine %d: Waiting for connection", id)
			// fmt.Println(msg)
			// conn, _ := cp.GetConnection()
			// msg = fmt.Sprintf("GoRoutine %d: Connection established", id)
			// fmt.Println(msg)
			// time.Sleep(5 * time.Second) // Siulate some work with the connection
			// msg = fmt.Sprintf("GoRoutine %d: Releasing connection", id)
			// fmt.Println(msg)
			// cp.SetConnection(conn)
		}(i)
	}
	wg.Wait()
	db.Close()
	duration := time.Since(startTime)
	fmt.Printf("Total time taken for %d connections: %v\n", count, duration)
}
