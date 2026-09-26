# Load balancer, DNS, VIP — revision notes

Video: https://www.youtube.com/watch?v=g_gKI2HCElk

Use this to recall: how a client picks a load balancer, what DNS is, Route 53 vs `8.8.8.8`, why there is no one global website database, the 65K myth, and what a virtual IP is.

Your Mac’s resolver in this setup was the **home router** (`192.168.1.1`), from DHCP — not a manually set `8.8.8.8`.

---

## A. Multiple load balancers

### Q1. If there are two LBs, how does the client know which one to connect to?

It usually **does not pick an LB by name**. It uses a **stable front door**: a hostname or one IP. Something else chooses the machine.

```text
client  →  name or VIP  →  one of the LBs  →  app servers
```

### Q2. What are the common front doors?

| Front door | What the client sees | Who chooses the LB |
|---|---|---|
| DNS name with several A records | `anubhav.lbs.com` → two IPs | OS / resolver (picks one IP) |
| Virtual IP (VIP) | one IP | whichever machine currently owns that IP |
| Anycast | one IP, many sites | the network (nearest/healthy path) |
| Smart client / sidecar | list or `localhost` | library or local proxy |

The app is configured with **the name** (or one VIP), not “LB1 vs LB2.”

### Q3. What happens if LB1 dies?

Depends on the front door:

- **Health-checked DNS** stops returning LB1’s IP (after TTL / health check).
- **VIP** moves to LB2. Clients still dial the same IP.
- **Client retry** tries the next IP from DNS.

---

## B. DNS (the phone book)

### Q4. What is DNS?

DNS turns a **name** into an **IP**. Machines send packets to IPs, not to `www.google.com`.

```text
www.google.com  →  DNS lookup  →  142.x.x.x  →  TCP/TLS to that IP
```

### Q5. What is a record? What is TTL?

- **A** = IPv4, **AAAA** = IPv6.
- **NS** = “these servers are the source of truth for this zone.”
- **CNAME** = this name is an alias for another name.
- **TTL** = how long a resolver may **cache** the answer before asking again.

### Q6. Recursive resolver vs authoritative server?

Two different jobs. Do not mix them.

| Role | Examples | Job |
|---|---|---|
| **Recursive resolver** | home router `192.168.1.1`, ISP DNS, `8.8.8.8`, `1.1.1.1` | Asks around **for you**, then **caches** |
| **Authoritative** | Route 53 (your zone), Google’s NS for `google.com` | **Owns** the records; source of truth |

Your laptop almost always talks only to the **resolver**. The resolver walks the tree.

### Q7. How does the client know which DNS to hit?

From **network config** (usually DHCP), not from the app.

- Home Wi‑Fi: often the **router** (`192.168.1.1`).
- You can set `8.8.8.8` / `1.1.1.1` manually.
- If two resolvers are listed, the OS tries the first, then the next on timeout.

**macOS note:** `networksetup -getdnsservers Wi-Fi` only shows **manual** DNS. If it says there aren’t any, DHCP DNS can still be working. Check with:

```bash
ipconfig getpacket en0 | grep domain_name_server
scutil --dns
dscacheutil -q host -a name google.com
```

`resolvectl` is Linux, not macOS.

---

## C. The lookup walk (no single global spreadsheet)

### Q8. Is there one central database of all websites?

**No.** DNS is a **tree of databases**. Each layer only knows the **next** owner.

```text
.  (root)                 who runs .com, .org, .in, …?
 └── com                  who runs google.com / lbs.com?  (TLD registry)
      └── google.com      Google’s nameservers
           └── www        the actual A/AAAA (or CNAME)
```

- **Root** does not know `www.google.com`.
- **`.com`** does not know every hostname at Google. It only knows **which NS own `google.com`**.
- **You** own `anubhav.lbs.com` only inside **your** zone.

### Q9. Walk for `www.google.com` from this Mac

```text
Mac
  → router 192.168.1.1  (resolver, or it forwards to ISP DNS)
    → root:        where's .com?
    → .com TLD:    where's google.com?
    → Google NS:   what's www.google.com?  → IP
```

There is **no “root 53.”** DNS uses **port 53**. The “13 roots” are 13 **identities** (`a.root-servers.net` … `m.root-servers.net`), each **anycasted** to many machines.

After the first success, **caches** (Mac, router, ISP) often answer later queries without hitting the roots.

### Q10. If I spin up two LBs named `anubhav.lbs.com`, how do `8.8.8.8` / `1.1.1.1` learn it?

They do **not** watch your cloud account. You must **publish** DNS:

1. Own `lbs.com` (or a subdomain).
2. Put the zone on some authoritative host (e.g. Route 53).
3. Add records: `anubhav.lbs.com` → LB IPs (or CNAME/alias to a cloud LB hostname).
4. TLD `.com` must list **your nameservers** for `lbs.com`.

On **first lookup**, a resolver walks root → `.com` → your NS and **caches**. If you never publish, **every** resolver returns NXDOMAIN (name does not exist). Creating LBs alone does not create the name.

`8.8.8.8` and `1.1.1.1` **never sync with each other**. Both ask the same official tree.

Cloud shortcut: AWS ALB already has a name like `xxx.elb.amazonaws.com`. You still add `anubhav.lbs.com` → that name in **your** zone.

---

## D. Route 53

### Q11. What is Route 53?

Amazon’s **DNS product** (plus health checks and traffic policies). Name joke: US “Route 66”, DNS **port 53**.

It is **not** the load balancer and **not** the `.com` registry.

### Q12. Is Route 53 an authoritative nameserver?

**Yes, for zones you host there.** It answers records you create. The `.com` registry is told: questions about `lbs.com` go to Route 53’s `ns-*.awsdns-*` servers.

`.com` itself is **not** Route 53. Google.com is **not** answered by your Route 53 account.

---

## E. Scale: the 65K myth and `8.8.8.8`

### Q13. A machine has ~65K ports. How does `8.8.8.8` serve the world?

**65K is port numbers (16-bit), not “max users on Earth.”**

- A TCP connection is `(src IP, src port, dest IP, dest port)`. One **client** talking to one server IP+port has ~64k source ports. That is a **client** limit, not Google’s user cap.
- A **server** on port 53 can have a huge number of clients; each client is a different `(client IP, client port)`.

Most classic DNS is **UDP**: one small question packet, one answer, no long-lived connection. One box can do a very large QPS.

**`8.8.8.8` is not one computer.** Same IP, **many data centers** (anycast). You hit a nearby cluster.

---

## F. Virtual IP (VIP) — shop number, not the phone

### Q14. What is a VIP, in one picture?

Two phones in a shop (two LBs). Customers should not know which phone is on. You publish **one shop number**.

- Today phone A rings for that number.
- Phone A dies → the shop number is moved to phone B.
- Customers still dial the **same number**.

That shop number is the **VIP**. Each phone’s personal number is a **real IP**.

```text
Clients  →  10.0.0.10   (VIP = shop number)
                │
         ┌──────┴──────┐
         ▼             ▼
   LB1 (on duty)   LB2 (takes VIP if LB1 dies)
   10.0.0.11       10.0.0.12     ← real IPs
```

You do not need ARP/VRRP/keepalived to *use* the idea. Those are just *how* the shop number is moved.

### Q15. VIP vs two DNS A records?

| | Two A records | One VIP |
|---|---|---|
| Client sees | two IPs | one IP |
| Who picks the LB | DNS / OS | who currently owns the VIP |
| Failover | wait for TTL / try other IP | VIP moves, same number |

DNS = how people find the shop **name** (`anubhav.lbs.com`). VIP = the shop **number** itself.

---

## G. One stack (put it together)

```text
App uses hostname
  → OS resolver (router / 8.8.8.8)          recursive
    → root → TLD → your NS (e.g. Route 53)  authoritative
      → one or more LB IPs, often a VIP
        → load balancer
          → app servers
```

| Layer | What is duplicated | Who chooses |
|---|---|---|
| Resolver | router, `8.8.8.8`, `1.1.1.1` | OS / DHCP |
| Authoritative NS | several `ns-*.awsdns-*` | resolver, after asking TLD |
| VIP / anycast | many machines, one service IP | network |
| Load balancers | LB1, LB2 | DNS IPs or VIP owner, then LB picks backends |

`localhost:3306` (e.g. connection pool demos) **does not** use this DNS path.

---

## H. Quick self-check

1. Does the client pick LB1 vs LB2? **No — it uses a name or VIP.**
2. Is Route 53 the same as `8.8.8.8`? **No — authoritative vs recursive.**
3. Must you register with Google DNS for `anubhav.lbs.com`? **No — publish in your zone; resolvers discover it.**
4. One database of all websites? **No — hierarchy.**
5. VIP = a machine’s home address? **No — a floating shop number.**
6. `8.8.8.8` = one box limited to 65K users? **No — UDP + anycast + many machines.**
