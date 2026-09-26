# Go `database/sql` Connection Pool — Revision Notes

Prototype: `connection_pool/main.go`  
Driver: `github.com/go-sql-driver/mysql`  
DSN: `root:@tcp(localhost:3306)/test`

Use this as a recap of what `*sql.DB` actually is, what opens a MySQL session, and why 50k goroutines explode the server.

---

## 1. Mental model (memorize this)

```
sql.Open  →  *sql.DB  = pool *handle* (no TCP yet)
                 │
                 ├── Exec / Query / Ping  → borrow a socket, run SQL, put it back
                 └── Conn                 → check out a socket and KEEP it until conn.Close()
```

| Call | Real MySQL TCP session? | Holds it after return? |
|---|---|---|
| `sql.Open` | No | n/a |
| `db.Ping` / `Exec` / `Query` | Yes, if pool has no free conn | No — returned to pool |
| `db.Conn(ctx)` | Yes, if needed | **Yes**, until `conn.Close()` |
| `db.Close()` | Closes the **whole pool** | n/a |
| `conn.Close()` | Returns **one** checkout to the pool | Does not close `*sql.DB` |

`*sql.DB` is a **connection pool**, not one connection. It is safe to share across goroutines. Create **one** per process (per DSN), not one per request.

---

## 2. Two experiment shapes

### A. One pool, many queries (normal app)

```go
db := sql.Open("mysql", dsn) // one handle
defer db.Close()

// N goroutines:
db.Exec("SELECT SLEEP(0.1)")
```

- All goroutines share sockets inside that pool.
- Concurrent in-flight queries can each need their **own** socket.
- Cap that with:

```go
db.SetMaxOpenConns(10)  // max live MySQL sessions from this pool
db.SetMaxIdleConns(10)  // keep some warm for reuse
```

Default `MaxOpenConns` is **0 = unlimited**. Then 50k overlapping `SLEEP`s ≈ 50k dials.

### B. Many pools, one query each (anti-pattern / “non-pooled” demo)

```go
// each goroutine:
db := sql.Open("mysql", dsn) // its own empty pool
db.Exec("SELECT ...")        // this goroutine's first real dial
db.Close()
```

- `Open` 50k times still means **zero** sockets until `Exec`.
- After `Exec`, you can have up to 50k independent sockets. Nothing is shared.

**Production: always A.** B is only to contrast “no reuse.”

---

## 3. `Exec` vs `Conn`

**`db.Exec(sql)`** — “run this statement.”

1. Take a socket from the pool (or dial).
2. Send SQL.
3. When the query finishes, put the socket back.

You never own a `*sql.Conn`. Lifetime = query duration.  
`SELECT SLEEP(0.1)` holds a session ~100ms. Invalid SQL (`selct ...`) fails immediately, so it does **not** hold the session.

**`db.Conn(ctx)`** — “give me a private line.”

1. Reserve one socket. Nobody else on that `*sql.DB` uses it until you release.
2. No SQL is sent by `Conn` itself.
3. App `time.Sleep` still counts as an **open MySQL session**.
4. `conn.Close()` returns it to the pool.

Use `Conn` when you need **session affinity**: temp tables, `SET`, locks, or a demo that holds connections without a query.

```
Exec:  borrow → query → release
Conn:  borrow → keep (your sleep / extra queries) → conn.Close()
```

Each `Exec` may get a **different** physical session. Do not store session state across two `Exec`s unless you used `Conn` or a `Tx`.

---

## 4. Pitfalls we actually hit

### `sql.Open` does not connect

`Open` only parses DSN and allocates `*sql.DB`. First failure against MySQL is on `Ping`/`Exec`/`Conn`, not `Open` (unless DSN is malformed).

### `db.Close()` in the loop kills the pool

`Close()` shuts the **handle**, not one checkout.

```go
db := sql.Open(...)          // once
for i := 0; i < n; i++ {
    db.Exec(...)
    db.Close()               // after i=0, pool is dead
}
```

Later `Exec`s return `sql: database is closed` and **never** reach MySQL. If you ignore `error`, the loop still “succeeds” in ~10ms. That time is **not** 50k connections.

Close **once**, after `wg.Wait()`.

### Timer without `wg.Wait()` is spawn time

If you `go func` and print `time.Since` without `Wait`, you measure goroutine creation, then `main` exits and kills work.

```go
wg.Add(n)
// launch goroutines, each defer wg.Done()
wg.Wait()
db.Close()
fmt.Println(time.Since(start))
```

### Ignoring `Exec` errors hides everything

Always:

```go
if _, err := db.Exec(...); err != nil {
    fmt.Println(id, err)
}
```

### Loop variable vs goroutine id

Print `id` (the argument), not `i`, unless you are on Go 1.22+ and sure about per-iteration loop vars.

### Typo SQL does not sleep

`selct SLEEP(0.1)` → syntax error, connection not held.  
`SELECT SLEEP(0.1)` → holds ~100ms.

### Function name vs reality

One `sql.Open` + many `Exec` = **pooled**.  
Many `sql.Open` + `Exec` = **non-pooled** (many handles).  
Logging “N connections” when you ran N queries is misleading.

---

## 5. Errors from the 50k burst

With one unlimited pool (or 50k pools) and concurrent `SLEEP`:

| Error | Meaning |
|---|---|
| `Error 1040: Too many connections` | TCP + handshake succeeded; MySQL rejected login (`max_connections`, often ~151) |
| `dial tcp 127.0.0.1:3306: connect: connection reset by peer` | Handshake never finished; listen backlog full, kernel/MySQL **RST** the SYN |
| `sql: database is closed` | You `Close()`d the `*sql.DB` already |

Log line like `49074 dial tcp ...` — **49074 is `i` / goroutine id**, not an error code.

**Fix for a pool demo:** `SetMaxOpenConns(N)`. Extra goroutines **wait** instead of dialing. With `N=1` and 50k × `SLEEP(0.1)`, wall time ≈ 5000s plus scheduling — slow, but no 1040/RST.

---

## 6. Pool knobs (client)

```go
db.SetMaxOpenConns(n)   // cap concurrent MySQL sessions from this *sql.DB
db.SetMaxIdleConns(n)   // idle sockets kept alive for reuse (default 2)
db.SetConnMaxLifetime(...)
db.SetConnMaxIdleTime(...)
```

Server side: `SHOW VARIABLES LIKE 'max_connections';`

Go cap should be **below** MySQL `max_connections` (leave room for CLI, replicas, other apps).

---

## 7. Tiny cheatsheet

```go
// RIGHT: one pool for the process
db, err := sql.Open("mysql", dsn)
db.SetMaxOpenConns(25)
db.SetMaxIdleConns(25)
defer db.Close()

var wg sync.WaitGroup
wg.Add(count)
for i := 0; i < count; i++ {
    go func(id int) {
        defer wg.Done()
        if _, err := db.Exec("SELECT SLEEP(0.1)"); err != nil {
            fmt.Println(id, err)
        }
    }(i)
}
wg.Wait()
```

```go
// HOLD a session (Conn), then release
conn, err := db.Conn(ctx)
// ... work; MySQL still counts this session ...
conn.Close() // back to pool
```

```go
// WRONG: Close per query on a shared db
db.Exec(...); db.Close()

// WRONG: Open per request in a real server
for each request { sql.Open(...); Exec; Close }

// WRONG: time the loop without wg.Wait()
```

---

## 8. What “correct” looked like in this prototype

**Pooled benchmark:** one `getConnection()`, shared `db`, `SetMaxOpenConns`, goroutines + `Exec`, `wg.Wait()`, then `db.Close()`.

**Non-pooled / hold-session benchmark:** `Open` (and optionally `Conn`) **inside** each goroutine, work, `conn.Close()` then `db.Close()` **that** handle, `wg.Wait()` in `main`. Expect 1040/RST if `count` ≫ `max_connections`.

**Handmade pool** (`chan *sql.Conn` of size 5): that is an explicit checkout queue on top of `database/sql`. Prefer `SetMaxOpenConns` unless you are implementing the exercise on purpose.

---

## 9. One-line interview answer

`sql.Open` creates a pool handle; connections are opened lazily. `Exec` borrows a connection for one query; `Conn` checks one out until `Close`. Share one `*sql.DB`, set `MaxOpenConns`, never `Open`/`Close` per request. Unlimited concurrent queries will hit MySQL `max_connections` (1040) or the accept queue (connection reset).
