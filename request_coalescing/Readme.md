# Request Coalescing / Single-Flight / Cache Stampede Prevention

## Overview
When multiple concurrent requests try to fetch the **same data** and the data is **not in cache**, they may all hit the database simultaneously.  
This can overload databases, increase latency, and cause cascading failures.

To prevent this, we ensure that:
- **Only one** goroutine performs the expensive fetch for a given key.
- **All other** goroutines wait for the result and reuse it.

This pattern is known as:
- Request Coalescing
- Thundering Herd Prevention
- Dogpile Prevention
- Single-Flight Fetching

## The Core Problem

### Without protection:
```
cache.get("blog:123") → miss
100 concurrent users call db.Get("blog:123")
DB overloaded → requests slow/fail
```

### With protection:
```
cache.get("blog:123") → miss
Request #1 fetches from DB
Requests #2..100 wait
Request #1 caches result and signals others
Everyone gets the same result
```

## Key Idea
We maintain a map that tracks **in-progress fetches**.

For each key:
- If data is already cached → return immediately
- If fetch for the key is *already in progress* → wait
- Otherwise → perform fetch, store result, notify others

## Pseudocode
```
if key in cache:
    return cache[key]

if key in sem_map:
    wait on sem_map[key]
    return res_map[key]

create semaphore for key
perform db fetch
cache result
signal waiting goroutines
return result
```

## Go Implementation (Manual Semaphore Version)

```go
type Cache struct {
    cache map[string]string
    sem   map[string]chan struct{}
    res   map[string]string
    mu    sync.Mutex
}

func (c *Cache) Get(key string) string {
    c.mu.Lock()
    if v, ok := c.cache[key]; ok {
        c.mu.Unlock();
        return v
    }

    if ch, ok := c.sem[key]; ok {
        c.mu.Unlock()
        <-ch
        c.mu.Lock()
        v := c.res[key]
        c.mu.Unlock()
        return v
    }

    ch := make(chan struct{})
    c.sem[key] = ch
    c.mu.Unlock()

    v := dbGet(key)

    c.mu.Lock()
    c.cache[key] = v
    c.res[key] = v
    close(ch)
    delete(c.sem, key)
    c.mu.Unlock()

    return v
}
```

## Why Unlock Before Waiting?
If a goroutine waits on `<-ch` **while holding the mutex**, the goroutine performing the DB fetch would block when trying to lock the mutex to store the result.  
This results in **deadlock**.

**Rule:** Never wait on a channel while holding a mutex.

## Recommended Production Approach: `singleflight`
Go provides a built-in solution:

```
import "golang.org/x/sync/singleflight"

var g singleflight.Group
var cache sync.Map

func Get(key string) string {
    if v, ok := cache.Load(key); ok {
        return v.(string)
    }

    v, _, _ := g.Do(key, func() (interface{}, error) {
        val := dbGet(key)
        cache.Store(key, val)
        return val, nil
    })

    return v.(string)
}
```

## When to Use This Pattern
Use when:
- Fetch is expensive (DB or external API)
- Data is cached
- Multiple requests may hit same key concurrently

Examples:
- User profiles
- Product pages
- Payment status checks
- Heavy API calls

## Summary
| Concept | Explanation |
|--------|-------------|
| Problem | Multiple requests hitting DB at same time |
| Solution | Only one fetch happens; others wait |
| Implementation | Semaphore / singleflight |
| Result | Lower DB load, stable systems |

## Key Takeaway
**Ensure only one goroutine performs expensive work for a given key, while all others wait and reuse the result.**
