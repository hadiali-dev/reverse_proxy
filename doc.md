# Reverse Proxy / Load Balancer — Build Spec (Go)

## Ground rules
- No `net/http/httputil.ReverseProxy`, no router libs, no LB/health-check libs.
- Allowed: `net`, `net/http` (as transport plumbing only), `sync`, `time`, `context`, `crypto/tls`, `log/slog`.
- You write: routing logic, backend selection, health checks, connection handling, shutdown logic.
- Every phase must run and be tested locally before moving to the next. Don't skip ahead — each phase's pain is the lesson.

---

## Phase 0 — Raw TCP warm-up (half a day)
**Goal:** stop thinking of `net/http` as magic.

Build a raw TCP echo server with `net.Listen("tcp", ...)` and `net.Conn`. Accept a connection, read bytes, write them back, close.

Then build a raw TCP *client* that connects to a real HTTP server (e.g. `curl -v example.com` to see the format) and manually writes a valid HTTP/1.1 request line + headers to the socket, then reads the raw response bytes.

**Why:** you need to see, once, that HTTP is just text over a socket. Everything after this phase should feel less like magic.

**Done when:** you can hand-write a GET request to google.com over raw TCP and print the response headers.

---

## Phase 1 — Dumb single-backend proxy
**Goal:** request goes in one side, comes out the other, unmodified logic.

- One listener on `:8080`.
- One fixed backend, e.g. `localhost:9001`.
- Your own `http.Handler` (implement `ServeHTTP` yourself — don't use `http.HandlerFunc` wrappers from a lib, write the struct + method).
- Inside `ServeHTTP`: open a connection to the backend yourself, forward the method/path/headers/body, read the backend's response, copy status/headers/body back to the original client.
- Use `io.Copy` for body streaming (this is fine — it's a primitive, not the logic).

**Concepts forced on you:**
- Which headers must NOT be blindly forwarded (`Connection`, `Transfer-Encoding`, hop-by-hop headers per RFC 7230 §6.1 — `Keep-Alive`, `Proxy-Authenticate`, `Proxy-Authorization`, `TE`, `Trailer`, `Upgrade`) vs which are safe (`Host`, `Content-Type`, etc.)
- Why you need to set `X-Forwarded-For` and `X-Forwarded-Host` — and why blindly trusting an incoming `X-Forwarded-For` is a security bug (spoofable, has to be appended-to not replaced-if-already-present)
- Streaming vs buffering the body (what breaks if you `ioutil.ReadAll` the body into memory first — large uploads, memory blowup)

**Test:** two terminals — dummy backend on `:9001` returning `"hello from 9001"`, curl `:8080`, confirm passthrough.

**Edge cases to handle before moving on:**
- Backend is down → proxy shouldn't crash, should return 502
- Backend is slow → what happens with no timeout set? (nothing, it hangs forever — this bites you in Phase 6)
- Client disconnects mid-request → does your goroutine leak?

---

## Phase 2 — Multiple backends + round robin
**Goal:** introduce the "pick a target" abstraction, and see why the naive version doesn't scale.

- Config: list of backend addresses (hardcode a `[]string` for now, config file comes in Phase 6).
- Write a `Backend` struct and a `Pool` type that owns the list + an index.
- Round robin: `atomic.AddUint64` on a counter, mod by pool size — NOT a mutex-guarded plain int (do the naive mutex version first if you want, then swap to atomic and understand why atomic is cheaper here: no lock contention, single word, CPU-level instruction).

**Concepts forced on you:**
- Race conditions: run naive `counter++` without atomic/mutex under concurrent load (`go test -race`, or hammer with `hey -c 50 -n 5000`), watch it produce duplicate/skipped indices. This is the single most valuable "aha" in the whole project — you'll actually see a race, not just read about one.
- Why round robin alone is naive (doesn't account for backend load/capacity — sets up Phase 4)

**Test:** 3 dummy backends on `:9001/9002/9003`, each printing its own port in the response. Fire 30 requests, confirm even distribution.

---

## Phase 3 — Health checking
**Goal:** the proxy must never route to a dead backend if it can avoid it. This is the phase that teaches real concurrency.

Two mechanisms, build both:

**Active health checks:**
- Background goroutine per backend, ticks every N seconds (`time.Ticker`), does a lightweight GET to a `/health` path (or your dummy backends' root).
- On failure: mark backend `Alive = false`.
- On success after being dead: mark `Alive = true` (recovery).

**Passive health checks:**
- If a live request to a backend fails (connection refused, timeout), immediately mark it dead — don't wait for the next active check tick.

**Concepts forced on you:**
- Shared mutable state across goroutines: the backend's `Alive` bool is written by the health-check goroutine AND read by every request-handling goroutine selecting a backend. This needs a `sync.RWMutex` (many readers picking a backend, occasional writer flipping alive state) — use `RWMutex` specifically and be able to explain why not a plain `Mutex`.
- Where exactly the lock goes: lock around the read/write of the field, NOT around the whole health-check HTTP call (locking during a slow network call is a classic junior mistake — it serializes everything waiting on that lock).
- Thundering herd on recovery: if a backend comes back and every goroutine immediately piles onto it, that's a real production failure mode. You don't have to solve it, but you should be able to name it (this is what "slow start" / gradual weight ramp-up exists for in real LBs).

**Test:** kill one dummy backend mid-run (Ctrl+C it), confirm proxy routes only to survivors within one health-check interval. Restart it, confirm it rejoins rotation. Run with `-race` — must be clean.

---

## Phase 4 — Load balancing strategies
**Goal:** round robin isn't the only algorithm, and the abstraction from Phase 2 gets tested here.

Implement at least:
- **Least connections** — track active connection count per backend (increment on request start, decrement on response written — careful with `defer` placement). Pick backend with fewest active.
- **Weighted round robin** — some backends get more traffic (simulates unequal server capacity).

Refactor backend selection behind an interface:
```go
type Strategy interface {
    Pick(backends []*Backend) *Backend
}
```
Swap strategies via config/flag.

**Concepts forced on you:**
- Interface design from real requirements, not upfront guessing — you'll notice Phase 2's design either supports this cleanly or needs rework. That rework *is* the lesson (this is what "your first design is wrong" actually feels like).
- Active-connection counting has the same race-condition shape as Phase 3 — reinforces it under a different metric.

---

## Phase 5 — Timeouts, retries, circuit breaker
**Goal:** resilience. This is where it stops being a toy.

- Set explicit timeouts: dial timeout, response header timeout, overall request timeout (`context.WithTimeout` passed through to the backend call).
- Retry logic: if a backend request fails (not client-caused), retry once against a *different* backend — but only for idempotent methods (GET/HEAD), never blindly retry a POST.
- Circuit breaker per backend: after N consecutive failures, stop sending traffic to it for a cooldown window even if health checks haven't caught up yet (states: closed → open → half-open, same shape as Warden's behavioral firewall logic — you've built this state machine before, recognize it).

**Concepts forced on you:**
- Why "retry" is dangerous by default (retry storms — a struggling backend gets hit harder by retries, making it worse; this is a real outage pattern, look up the AWS/Google SRE writeups on retry storms if you want the "why this matters" context)
- Timeout propagation: what happens to the backend connection if the client gives up — does your code cancel it via context, or does the goroutine keep running uselessly?

---

## Phase 6 — Config, logging, observability
**Goal:** make it operable, not just functional. This is the difference an interviewer actually notices.

- Config file (YAML or JSON) instead of hardcoded backend list: listen address, backend list, health check interval, strategy choice, timeouts.
- Structured logging (`log/slog`): every request logs method, path, backend chosen, latency, status — as structured fields, not `fmt.Println`.
- Expose a `/metrics` or `/status` endpoint on the proxy itself showing: per-backend alive/dead, active connections, request counts. (Prometheus format if you want to go further, but a plain JSON status endpoint is enough to demonstrate the idea.)

**Concepts forced on you:**
- Separating control plane (config/status) from data plane (request forwarding) — same split real load balancers (nginx, envoy, haproxy) all have.

---

## Phase 7 — TLS termination
**Goal:** proxy accepts HTTPS from clients, talks plain HTTP to backends internally (standard pattern — backends live in a trusted network, TLS overhead only paid once at the edge).

- Generate a self-signed cert (`openssl req -x509 ...` or Go's `crypto/tls` cert generation) for local testing.
- `http.Server{TLSConfig: ...}` — using `crypto/tls` here is correct, don't hand-roll TLS.

**Concepts forced on you:**
- Why TLS termination happens at the edge, not per-backend, in most real architectures
- The difference between this and end-to-end TLS (when would you need TLS to the backend too — e.g. zero-trust/mTLS internal networks)

---

## Phase 8 — Graceful shutdown
**Goal:** the thing almost nobody builds, and the thing that separates "works on my machine" from "safe to deploy."

- Listen for `SIGTERM`/`SIGINT` (`os/signal`).
- On shutdown signal: stop accepting new connections, let in-flight requests finish (with a max drain deadline), THEN exit.
- Use `http.Server.Shutdown(ctx)` (standard library — this is transport plumbing, fair to use) but you must understand and be able to explain what it does under the hood (stops listener, waits for active handlers to return, respects context deadline).

**Concepts forced on you:**
- Why abrupt kill = dropped requests = the actual cause of "5xx spike during deploy" that you've probably seen in real systems
- Connection draining is exactly what happens during every rolling deploy / pod termination in Kubernetes — you're building the mechanism `preStop` hooks depend on

---

## Stretch goals (optional, pick based on what you want to signal)
- **WebSocket proxying** — requires handling the `Upgrade` header correctly and switching to raw bidirectional byte copying instead of request/response. Good if you want to show you understand protocol upgrades.
- **Sticky sessions** (session affinity via cookie) — good if you want to connect this to your auth-system project.
- **Rate limiting per client IP** — reuses the token bucket concept from the original project list. Natural pairing.
- **HTTP/2 support** — deeper, mostly about understanding multiplexed streams over one connection.

---

## What this teaches, mapped to what interviewers actually probe
| Phase | Interview topic it answers |
|---|---|
| 0–1 | "Explain what happens when you type a URL and hit enter" / HTTP fundamentals |
| 2 | Race conditions, atomic vs mutex, why `go test -race` exists |
| 3 | Concurrent state management, RWMutex reasoning, failure detection design |
| 4 | System design: load balancing algorithms, trade-offs |
| 5 | Resilience patterns, retry storms, circuit breakers (you can now speak to this from two projects — Warden and this) |
| 6 | Observability, operability — "how would you debug this in production" |
| 7 | TLS, edge vs internal network trust boundaries |
| 8 | Deployment safety, what actually happens during a rolling restart/k8s pod termination |

That table is your interview prep, not just a build log — when someone asks "tell me about a project," you walk this table, not a feature list.

## Testing checklist (do this at every phase, not just at the end)
- `go test -race ./...` clean, always
- `hey -c 50 -n 5000 http://localhost:8080/` (or `ab`) for load
- Manually kill/restart a backend mid-load-test
- `curl -v` to inspect actual headers being forwarded — check hop-by-hop headers are stripped, `X-Forwarded-For` is correct

## Suggested pace
Phases 0–2: 2–3 days. Phase 3: 2–3 days (don't rush this one). Phase 4: 2 days. Phase 5: 3 days. Phase 6: 1–2 days. Phase 7–8: 1 day each. Roughly 3 weeks at a realistic pace alongside job applications.