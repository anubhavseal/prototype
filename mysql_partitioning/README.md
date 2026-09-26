# MySQL Table Partitioning — Revision FAQ

Use this when you need to recall the conversation: what partitioning is, RANGE vs HASH/KEY, why unique keys must include the partition column, why `WHERE id = ?` scans every partition, and how HASH can reduce write contention **without** changing row locks.

InnoDB is assumed throughout (MySQL partitioned tables use InnoDB). Indexes on partitioned InnoDB tables are **local** (one index per partition). There is no global secondary index.

Picture a table as **folders** (partitions). Each folder is a real smaller table: its own data, its own B-tree indexes, its own stats.

---

## A. What partitioning is

### Q1. What does partitioning a table do?

It keeps **one logical table** (`orders`) but stores rows in **several physical pieces** (partitions). App SQL still says `FROM orders`. The engine only opens the pieces it needs when it can.

```text
orders (what the app sees)
  ├── p0   folder / notebook
  ├── p1
  └── ...
```

### Q2. How does that help?

Four practical wins, not magic:

1. **Less work per query** if the `WHERE` includes the partition key. Engine skips other folders (**pruning**).
2. **Cheaper maintenance.** Drop last year with `DROP PARTITION` instead of a giant `DELETE`. Rebuild/analyze one slice.
3. **Write spread (HASH/KEY).** Inserts for different keys hit different B-trees, so they do not all slam one “last page.” See section E.
4. **Operational isolation.** One bloated partition is not “rewrite the whole table.”

It is **not** a substitute for a good index. A query **without** the partition key may still open **every** partition.

### Q3. Why are indexes and stats “smaller per partition”?

Because each partition is its own relation.

- An index is a sorted tree over **the rows in that folder**, not the whole table. 1B-row table → one huge B-tree. 12 monthly folders of ~80M → 12 smaller B-trees. Queries that prune to one month walk a smaller tree.
- Planner stats (row counts, histograms) are computed **per partition**, so they describe a smaller set and `ANALYZE` can refresh one slice.

**Caveat:** Total disk for all indexes together can be about the same (or a bit more). The win is **the piece you touch** is smaller — not “indexes vanished.”

### Q4. Local index vs global index?

| | Local | Global |
|---|---|---|
| What | One B-tree **per partition** | One B-tree over **all** rows |
| Size | Shrinks with the partition | Stays large after you partition |
| Uniqueness across the whole table | Each tree only sees its folder | One tree sees everyone |

**MySQL / Postgres:** you get **local** indexes. Oracle is where `LOCAL` vs `GLOBAL` is a real choice.

That is why MySQL cannot enforce `UNIQUE(id)` if `id` is not enough to know the folder: there is no global unique index watching every partition.

---

## B. Ways to partition in MySQL

### Q5. What types exist?

Four families, plus variants.

| Type | Row goes to a folder by… | Typical use |
|---|---|---|
| **RANGE** | value falls in a range (`YEAR(created_at) < 2026`) | Time data, drop old months |
| **LIST** | value is in an explicit list (`region_id IN (1,2)`) | Known enums: region, status |
| **HASH** | `MOD(integer_expr, N)` | Even spread by `user_id` |
| **KEY** | MySQL hashes the column(s) for you | Same as HASH, but column need not be int (UUID) |

Variants:

- **`RANGE COLUMNS` / `LIST COLUMNS`** — multiple columns, tuple compare, no expressions. Can use `DATE`, strings, ints.
- **`LINEAR HASH` / `LINEAR KEY`** — adding/removing partitions moves fewer rows; spread is less even.
- **Subpartitioning** — only RANGE/LIST folders, further split with HASH or KEY. Same number of subpartitions in every folder.

### Q6. The InnoDB rule that always applies

Every **unique** key (including the primary key) must **include the partitioning columns**.

```text
PARTITION BY HASH (user_id)
PRIMARY KEY (id)              -- ILLEGAL
PRIMARY KEY (id, user_id)     -- OK
```

Max 8192 partitions (counting subpartitions) in MySQL 8.

### Q7. RANGE vs HASH in one sentence

- **RANGE:** “put January in folder Jan; later **drop** folder Jan.” Inserts **today** all go to the **latest** folder (write hotspot, cheap archive).
- **HASH/KEY:** “spread users across N folders.” You **cannot** drop “last year.” Inserts spread. Lookups prune only if `WHERE` has the hashed column.

---

## C. HASH / KEY — when it is actually useful

### Q8. What is the real use case of HASH(user_id)?

Not “make `SELECT WHERE id = ?` fast.”

The app already always has a **well-distributed** key (`user_id` / `tenant_id` / `session_id`). Inserts are heavy. Pain is **one giant clustered index** (or RANGE’s **current-month** folder) getting all new rows.

HASH uses the **same routing formula** as sharding (`user_id % N`) but still **one MySQL**. Real sharding is **new databases / machines**. See Q10a.

Use HASH/KEY when:

- Almost every OLTP query is `WHERE user_id = ?` (or `session_id = ?`).
- The key is high-cardinality and even (not 3 tenants with one huge whale).
- You care about insert spread, **not** dropping 2019 by partition.

Do not use HASH when:

- Workload is time/metrics → RANGE (or separate archive tables).
- You look up by surrogate `id` only → HASH makes that **worse** (all folders).
- You need `id` unique **by itself** in the database → fights Q6.

### Q9. HASH vs KEY example?

```sql
-- integer shard key, you hash with MOD
PRIMARY KEY (id, user_id)
PARTITION BY HASH (user_id) PARTITIONS 8;
-- user_id 16 → folder 16 % 8 = 0

-- UUID / string shard key; MySQL hashes
PRIMARY KEY (session_id)
PARTITION BY KEY (session_id) PARTITIONS 8;
```

`HASH` needs an **integer** expression. `KEY` does not.

`SELECT ... WHERE user_id = 16` → only folder 0.  
`SELECT ... WHERE id = 999` → **all 8 folders**.

### Q10. Is HASH just “B-tree rebalancing”?

No. Most inserts **do not** rebalance the whole tree. See Q20.

HASH’s write win is mainly **more last pages** so insert **page waits** are spread (section E). Reads can still prune on `user_id`.

### Q10a. Is sharding the same as partitioning?

**Same idea, different place.** Both split rows by a key (`user_id % N`). They are **not** the same thing.

| | Partitioning | Sharding |
|---|---|---|
| What you split | One **table** into folders | One **dataset** into several **databases / servers** |
| Who routes | **MySQL** (`PARTITION BY ...`) | **App or proxy** (pick host from `user_id`) |
| App connection | Still **one** DSN, `FROM orders` | Often **8 pools / 8 DSNs**, or a shard router |
| Unique `id`, joins, transactions | One InnoDB, one transaction | Across shards: hard (no cheap cross-shard JOIN / FK) |
| Failure / capacity | One machine dies → all folders gone | One shard dies → only that slice; you can add **machines** |
| Example | `PARTITION BY HASH (user_id) PARTITIONS 8` | `orders` on `db0`…`db7`, still `user_id % 8` |

```text
Partitioning (one MySQL):
  app  →  one database  →  orders { p0, p1, … p7 }

Sharding (many MySQLs):
  app  →  user_id % 8  →  mysql-0 / mysql-1 / … / mysql-7
                         each has its own orders table
```

People say HASH partitioning is “sharding **inside** one database” because the **formula** is the same. The **new database** wording is what **real sharding** is: a new server (or at least a new schema/instance), not another folder in the same table.

You partition when one instance is enough but the table/B-tree is huge. You shard when **one instance cannot hold the load or the data**.

---

## D. Unique keys vs pruning (the confusing part)

Memorize: **same extra column, two jobs, two moments.**

| Job | Question | Needs partition column… |
|---|---|---|
| **Uniqueness** (INSERT) | May this row duplicate a key? | **In the UNIQUE/PK definition** |
| **Pruning** (SELECT) | Which folders do I open? | **In the WHERE clause** |

Putting `user_id` in the PK does **not** put `user_id` in a query that only has `id`.

### Q11. Why must unique keys include the partition columns? (integrity, not pruning)

Each folder has **its own** unique index. Folders **do not look at each other**.

If PK were only `id` and folders were by `user_id`:

```text
INSERT (id=5, user_id=1) → folder 1, unique(id) in folder 1: OK
INSERT (id=5, user_id=2) → folder 2, unique(id) in folder 2: OK
```

Two rows with `id = 5`. The PK is a lie. That is why MySQL refuses that schema.

Fix: `PRIMARY KEY (id, user_id)` when you partition by `user_id`.

```text
(5, user 1) can only live in folder 1
Second (5, user 1) → same folder → rejected
(5, user 2) is a different pair → allowed
```

The database no longer promises “`id` is unique everywhere.” It promises “this **pair** is unique,” and that pair has only **one** possible folder, so that folder can enforce it **alone**.

This rule is **not** so that `WHERE id = 5` opens one folder.

### Q12. Then why does `WHERE id = 5` open all 8 folders?

Pruning needs the **partition expression’s inputs**.

`PARTITION BY HASH (user_id)` → engine must know `user_id` to compute `user_id % 8`. The SELECT did not give `user_id`. So it opens every folder and looks for `id = 5`.

| Query | Knows the folder? |
|---|---|
| `WHERE user_id = 16` | Yes |
| `WHERE id = 5 AND user_id = 16` | Yes |
| `WHERE id = 5` | No → all 8 |

### Q13. Can that query return multiple rows with `id = 5`?

**Yes, if those rows exist.** With `PRIMARY KEY (id, user_id)`, `id` is **not** unique.

```text
(id=5, user_id=1) folder 1
(id=5, user_id=2) folder 2
```

`WHERE id = 5` can return **two** rows.

If your **app** never reuses `id` (AUTO_INCREMENT, UUID, snowflake), you usually get 0 or 1 row. The **database does not enforce that**. A second insert of `id=5` for another user is legal.

`WHERE id = 5 AND user_id = 1` is at most one row (full PK) and prunes.

### Q14. RANGE with `PRIMARY KEY (id, created_at)` — isn’t `id` unique then?

Same rule. Uniqueness is the **pair**, not each column.

| Two rows | Allowed? |
|---|---|
| Same `created_at`, different `id` | Yes (normal: many orders in one second). Same RANGE folder. |
| Same `id`, different `created_at` | Yes — **even in the same month folder**. Local PK sees two different pairs. |
| Same `id` **and** same `created_at` | **No.** Duplicate PK. |

RANGE uses `created_at` only to **pick the folder** (January vs February). It does not make `created_at` unique or `id` unique.

`WHERE id = 5` still opens all folders and can return several rows if that `id` was stored with different timestamps.

If you need `PRIMARY KEY (id)` alone, you **cannot** partition by `created_at` in InnoDB (`created_at` missing from the unique key).

### Q15. Are we only safe because the DB does not generate duplicate ids?

**For “`id` looks unique” — yes.** That safety is the **id generator**, not the PK and not partitioning.

| Who | Guarantees |
|---|---|
| PK `(id, created_at)` or `(id, user_id)` | This **pair** never repeats |
| AUTO_INCREMENT / UUID / snowflake | **Usually** `id` never repeats |
| Partitioning | Neither of the above; only which folder |

You are **not** allowing the exact same pair twice. You **are** allowing the same `id` with a **different** `user_id` / `created_at` unless the generator never reuses `id`.

Typical inserts: AUTO_INCREMENT gives 1, 2, 3…; many rows can share the same `created_at`. That is fine.

---

## E. Locks, pages, HASH contention

### Q16. Does a write lock the whole table or just the row?

**InnoDB DML: row locks**, not a table lock.

- `UPDATE`/`DELETE` one row → that row (in `REPEATABLE READ`, often **gap / next-key** locks nearby too).
- `INSERT` → the new row, maybe a gap.
- Two different rows → row locks do **not** wait on each other.

Whole-table-ish cases: `LOCK TABLES`, DDL (`ALTER`, `DROP PARTITION`), MyISAM. `AUTO_INCREMENT` uses a short **table-wide counter mutex** (not “lock every row”).

**Partitioning does not change lock grain.** You still lock **rows**. HASH is not “lock one partition instead of the table.”

### Q17. If it is only a row lock, how does HASH(user_id) reduce write contention?

Different `user_id`s already **do not** row-lock wait. HASH is not “smaller row locks.”

Rows live on **pages** (~16KB). To change a page, a thread must **hold that page** for a moment (page latch). **One writer at a time per page**, even for **different rows on that page**.

With `PRIMARY KEY (id, user_id)` and growing `id`, every insert is the **largest** key → every insert goes to the **last page** of **one** notebook.

```text
One notebook:  [old pages] [LAST PAGE ← all new ids fight here]
```

Different orders, different row locks, **same last page**. Under huge insert rate that page becomes a queue.

HASH = **8 notebooks, 8 last pages**. Same insert rate, ~1/8 the fights per tail.

```text
Folder 0 last page ← some users
Folder 1 last page ← other users
...
```

HASH does **not** help: two writers on the **same row**; the global AUTO_INCREMENT counter; one whale `user_id` (still one folder).

If the PK **already starts with `user_id`**, inserts for different users already land on **different pages** in one tree. HASH helps much less. The classic hotspot is an **append-only `id` as the first PK column**.

### Q18. Does every insert rebalance the B-tree?

**No.**

| Almost every insert | Sometimes | Rare |
|---|---|---|
| Latch last page, write the row, unlatch | Page **full** → **split** that page (local) | Splits reach the root → tree gets **taller** |

Contention you feel at high QPS is usually **waiting for the last page**, not a full rebalance every INSERT.

### Q19. Is a page the same as a disk block?

**No.**

- **InnoDB page:** database unit, usually **16KB**. Buffer pool, latches, splits = pages.
- **Disk / FS block:** often **4KB**. One page = several blocks (e.g. 4×4KB).

“Everyone fights the last page” means that **16KB InnoDB page in memory**, not one disk sector.

### Q20. Two writes, different rows, same page — must one wait until the other **finishes the SQL**?

They wait for the **page**, not for the **whole transaction**.

Two clocks:

1. **SQL / transaction** — until `COMMIT`. Can be long. **Row lock** is held this long.
2. **Page** — change the 16KB sheet. Tiny. Then **release the page** even if the transaction is still open.

**Wrong:** A holds the page until COMMIT; B cannot UPDATE until A commits.  
**Right:** A grabs page, changes its row, **puts the page down**; B can change the same page **before A commits**. They cannot both change the page in the same nanosecond.

```text
A: [row lock on row1 ======== until COMMIT ========]
A:     [page]     ← short, then page is FREE

B: [row lock on row2 ======== until COMMIT ========]
B:           [page]  ← after A's page, not after A's COMMIT
```

Book analogy:

- **Page latch** = take the book, write one word, **put the book down**. Next person writes immediately.
- **Row lock** = sticky note on **your** sentence until COMMIT. Others can still write **other** sentences.

| Situation | Wait on | Until |
|---|---|---|
| Same row | Row lock | Other **COMMIT** (can be long) |
| Different rows, same page | Page | Other **finishes the page write** (tiny) |
| Different pages | Neither of those | — |

“Only one write across all rows on that page” is true **at the page-latch moment**. It is **not** “only one writer per page until their SQL fully finishes.”

That is why HASH matters under **huge insert rate** (thousands of tiny last-page waits), not because two random updates feel like a table lock.

---

## F. Cheat sheet

```text
Partition = extra folders in **one** table / one MySQL. App still sees one table.
Shard     = extra **databases/servers**. App (or proxy) picks which MySQL. Same formula, not the same thing.

RANGE / LIST  → prune + drop by meaning (time, region). Latest RANGE folder is a write hotspot.
HASH / KEY    → even spread. Prune only if WHERE has the hash/key column. No “drop 2019.”

Indexes on MySQL partitions = local (per folder).
UNIQUE must include partition columns so a folder can enforce uniqueness alone.
That does not make WHERE id = ? use one folder.

PK (id, user_id)  → pair unique, id not necessarily unique.
Pruning             → WHERE must supply user_id / created_at / whatever the partition expr uses.

InnoDB writes lock rows, not the table.
Same page, different rows: serialize on the page for a moment, not until COMMIT.
HASH write win = more last pages (page latches), not weaker row locks.
Most inserts do not rebalance the whole B-tree.

Page (16KB InnoDB) ≠ disk block (often 4KB).
```

**Pick RANGE** for time and cheap drop. **Pick HASH/KEY** when OLTP is always by a even shard key and the clustered index would otherwise append to one last page. **Skip partitioning** if you need globally unique `id` as the only PK and also want to partition by something else — InnoDB will not let you.
