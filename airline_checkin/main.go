package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type User struct {
	id   int
	name string
}

type Trip struct {
	id   int
	name string
}

type Seat struct {
	id      int
	seat_id string
	trip_id int
	user_id sql.NullInt64
}

func getConnection() (*sql.DB, error) {
	db, err := sql.Open("mysql", "root:@tcp(localhost:3306)/airline")
	if err != nil {
		return nil, err
	}
	return db, nil
}

func seedUsers() {
	db, err := getConnection()
	if err != nil {
		log.Default().Printf("db connection error: %v", err)
		return
	}
	defer db.Close()

	stmt, err := db.Prepare("INSERT INTO users (name) VALUES (?)")
	if err != nil {
		log.Default().Printf("prepare error: %v", err)
		return
	}
	defer stmt.Close()

	for i := 0; i < 120; i++ {
		// generate simple varied names without extra imports: useraa, userab, ...
		a := 'a' + rune((i/26)%26)
		b := 'a' + rune(i%26)
		name := "user" + string(a) + string(b)

		if _, err = stmt.Exec(name); err != nil {
			log.Default().Printf("insert error for %s: %v", name, err)
		}
	}
}

func seedSeats() {
	db, err := getConnection()
	if err != nil {
		log.Default().Printf("db connection error: %v", err)
		return
	}
	defer db.Close()

	// Insert seat_id and trip_id. Adjust tripID if you have multiple trips.
	const tripID = 1

	stmt, err := db.Prepare("INSERT INTO seats (seat_id, trip_id) VALUES (?, ?)")
	if err != nil {
		log.Default().Printf("prepare error: %v", err)
		return
	}
	defer stmt.Close()

	letters := []rune{'A', 'B', 'C', 'D', 'E', 'F'}
	rows := 20 // 20 rows x 6 seats = 120 seats
	for r := 1; r <= rows; r++ {
		for _, l := range letters {
			seatID := fmt.Sprintf("%d%c", r, l)
			if _, err := stmt.Exec(seatID, tripID); err != nil {
				log.Default().Printf("insert error for seat %s: %v", seatID, err)
			}
		}
	}
}

func deAllocateSeats() {
	db, err := getConnection()
	if err != nil {
		log.Default().Printf("db connection error: %v", err)
		return
	}
	defer db.Close()

	// adjust tripID if you want to target a different trip; remove WHERE clause to clear all trips
	const tripID = 1

	res, err := db.Exec("UPDATE seats SET user_id = NULL WHERE trip_id = ?", tripID)
	if err != nil {
		log.Default().Printf("update error: %v", err)
		return
	}
	if n, err := res.RowsAffected(); err == nil {
		log.Default().Printf("deallocated %d seats for trip %d", n, tripID)
	}
}

func assignSeats(user_id int, wg *sync.WaitGroup) {
	defer wg.Done() // Ensure Done is called even if there's an error
	db, err := getConnection()
	if err != nil {
		log.Default().Printf("db connection error: %v", err)
		return
	}
	defer db.Close()

	opt := &sql.TxOptions{}
	seat := &Seat{}
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, opt)
	if err != nil {
		log.Default().Printf("transaction begin error: %v", err)
		return
	}
	// query := "SELECT * FROM seats WHERE user_id IS NULL ORDER BY id LIMIT 1"
	// query := "SELECT * FROM seats WHERE user_id IS NULL ORDER BY id LIMIT 1 FOR UPDATE"

	query := "SELECT * FROM seats WHERE user_id IS NULL ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED"
	err = tx.QueryRowContext(ctx, query).Scan(&seat.id, &seat.seat_id, &seat.trip_id, &seat.user_id)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Default().Printf("query error: %v", err)
		}
		tx.Rollback() // Rollback if there's an error
		return
	}

	_, err = tx.ExecContext(ctx, "UPDATE seats SET user_id = ? WHERE id = ?", user_id, seat.id)
	if err != nil {
		log.Default().Printf("update error: %v", err)
		tx.Rollback() // Rollback if there's an error
		return
	}

	log.Default().Printf("User %d assigned to seat %s", user_id, seat.seat_id)

	if err = tx.Commit(); err != nil {
		log.Default().Printf("transaction commit error: %v", err)
	}
}

func main() {
	deAllocateSeats()
	startTime := time.Now()
	wg := &sync.WaitGroup{}
	for i := 1; i <= 120; i++ {
		wg.Add(1)
		go assignSeats(i, wg)
	}
	wg.Wait()
	duration := time.Since(startTime)
	log.Default().Println("Duration %v", duration)
}
