# Research: load generator push ceiling

Context: ticket `.scratch/performance-scalability/issues/01-load-generator-push-ceiling.md`
asks whether the load source can push past the logger's knee, and what the
harness should publish with if it can't. The dev publisher today is
`scripts/publish_messages.py` — Python paho-mqtt 2.x, three topics, one
`wait_for_publish(timeout=5)` per message, `time.sleep(1)`, QoS 0 default: a
3 msg/s trickle whose rate proves nothing about the ceiling.

All claims below are traced to primary sources: paho-mqtt Python docs and
GitHub issues/PRs, paho Go client README, the official mosquitto.conf man page,
mosquitto's issue tracker, and first-party benchmark harnesses run by the tool
authors. Per-claim labels: **(direct)** = stated by the cited source;
**(interpretation)** = researcher reading of the source; **(inference)** =
researcher conclusion not stated by any source.

## 1. Python paho-mqtt on localhost, QoS 0

### What the client guarantees (and what it does not)

- `loop_start()` "start[s] a new thread to process network traffic … under the
  hood, this will call `loop_forever` in a thread"; the main thread is freed for
  other work <https://eclipse.dev/paho/files/paho.mqtt.python/html/client.html>
  **(direct)**.
- `publish()` returns an `MQTTMessageInfo`; success of `publish()` does **not**
  mean the bytes were sent — the docs are explicit that for QoS 0 `on_publish`
  "simply means that the message has left the client. This callback is important
  because even if the `publish()` call returns success, it does not always mean
  that the message has been sent"
  <https://eclipse.dev/paho/files/paho.mqtt.python/html/client.html> **(direct)**.
- `wait_for_publish()` blocks on a `threading.Condition` until `_published` is
  set. For QoS 0 that flag is set in `_packet_write()` only *after the network
  loop thread has written the PUBLISH packet to the socket*; for QoS 1/2 it is
  set when PUBACK/PUBCOMP arrives
  <https://github.com/eclipse-paho/paho.mqtt.python/blob/master/src/paho/mqtt/client.py>
  **(direct)**. So a per-message `wait_for_publish()` at QoS 0 costs one
  cross-thread condition round trip per message (publisher thread sleeps, loop
  thread notifies) and serialises the publisher behind the loop thread's socket
  writes; fire-and-forget lets the loop thread write packets back to back.
  **(inference)** from that code.
- For QoS 0 `publish()` bypasses `_out_messages` entirely and queues the raw
  packet directly; the client-side `max_queued_messages` default is `0`
  (unlimited), so a fast QoS 0 publisher does not hit a client-side queue-full
  error — it competes on CPU with the loop thread instead
  <https://github.com/eclipse-paho/paho.mqtt.python/blob/master/src/paho/mqtt/client.py>
  **(direct)**.
- `wait_for_publish()` can deadlock if called from inside a network-loop
  callback: paho maintainer MattBrittan on issue #875 — "You are calling
  `wait_for_publish`, which will block until the message is published, but as
  this blocks the network loop the message will not be sent (so you have a
  deadlock)"
  <https://github.com/eclipse-paho/paho.mqtt.python/issues/875> **(direct)**.
  Stalls of `wait_for_publish` in multi-threaded publishers are also reported in
  issues #598 <https://github.com/eclipse-paho/paho.mqtt.python/issues/598> and
  #354 <https://github.com/eclipse-paho/paho.mqtt.python/issues/354> **(direct,
  user reports)**. The current script only waits from the main thread, so this
  hazard does not apply to it as written. **(interpretation)**

### Measured single-client throughput (localhost, QoS 0)

The most controlled first-party evidence is `mqtt-python-client-bench`
(<https://github.com/yoch/mqtt-python-client-bench>, its author also filed the
benchmark numbers in paho PR #932). Its live report
<https://yoch.github.io/mqtt-python-client-bench/> (campaign dated 2026-10-05,
Intel i7-3770, `eclipse-mosquitto:2.1.2-alpine` pinned to one core, 256-byte
payloads, every count cross-checked against the broker's `$SYS` counters and a
neutral C peer) reports publish capacity, best first:

| Python client (2.1.0-era)      | QoS 0 msgs/s        | QoS 1 msgs/s (64 in flight, PUBACK) |
|--------------------------------|---------------------|-------------------------------------|
| mqttium 1.2.0 (asyncio)        | ≥ 78,053 (broker bound) | 25,725                          |
| gmqtt 0.7.0 (asyncio)          | ≥ 45,331 (broker bound) | 24,383                          |
| zmqtt 0.2.4 (asyncio)          | 27,329              | 18,470                              |
| awscrt 0.37.0 (native)         | 16,380              | 24,445                              |
| **paho 2.1.0 (threaded)**      | **12,595**          | **10,821**                          |
| aiomqtt 2.5.1 (asyncio/paho)   | 12,552              | 10,768                              |
| amqtt 0.12.1 (asyncio)         | 11,885              | 8,837                               |

**(direct)** — all values from the report's publish-capacity table.

Key readings from the same runs:

- **paho is client-CPU-bound far below the broker**: at 12.6k msg/s QoS 0 the
  paho worker burns 72–74 µs user + 36 µs sys ≈ **110 µs CPU per message** and
  137.7 % of a core across its threads (report run detail) **(direct)**. The
  broker's own C→C ceiling on that box was **78,269 msgs/s**
  (report header) **(direct)** — paho reaches only ~16 % of it.
- Cross-checks of the same harness family: PR #932's table of *stock* paho-mqtt
  2.1.0 (loopback, "Mosquitto 2.0.20, Python 3.12, localhost") shows
  `pub_qos_sweep_telemetry` QoS 0 at **9.5–9.6k msg/s** and a 256-byte QoS 0
  sweep at **10.0k msg/s**
  <https://github.com/eclipse-paho/paho.mqtt.python/pull/932> **(direct)**; the
  harness README's v1→v2 cross-validation mentions paho moving "from 11.3k to
  11.7k" QoS 0 msgs/s
  <https://github.com/yoch/mqtt-python-client-bench> **(direct)**.
- **Practical envelope: one Python paho process ≈ 10–12.6k msg/s QoS 0, ≈ 10.8k
  QoS 1** (three independent measurements, same harness family) **(inference
  from direct numbers)**. The spread (9.5k/11.7k/12.6k) reflects different
  scenarios/campaigns, not a contradiction — see §5.
- That paho is the *client's* bottleneck and not Python in general: gmqtt and
  mqttium are pure-Python asyncio clients reaching 45–78k msg/s in the same
  harness (table above) **(direct)**. paho's thread + per-message machinery
  costs ~4–6× what a tuned asyncio client achieves **(interpretation)**.

### wait_for_publish per message, the loop thread, GIL

- The docs' drive-shape note says each library "is driven the fastest way its
  own API allows" — paho's shape is "sync: from a plain thread"
  <https://github.com/yoch/mqtt-python-client-bench> **(direct)** — so ~12.6k
  msg/s *is* paho's fastest-admissible single-client rate in that harness
  **(interpretation)**. Whether the adapter literally calls `wait_for_publish()`
  per message is not stated in the harness docs (README/SCENARIOS.md do not say)
  **(missing evidence)**.
- No source isolates the µs cost of one `wait_for_publish()` call **(missing
  evidence)**. What is documented: PR #932's unmerged perf branch reworks
  exactly this path ("Threaded-loop deadline-driven waits, native socketpair
  wakeups", "Publish write-path and ACK/inflight refill batching") and measures
  +36–67 % on stock QoS 0 publish points — i.e. the stock wait/write path
  leaves measurable throughput on the table
  <https://github.com/eclipse-paho/paho.mqtt.python/pull/932> **(direct + interpretation)**.
- GIL: CPython docs — "a multi-threaded Python program effectively only uses
  one CPU, due to the insistence that (almost) all Python code can only run
  while the GIL is held", and the recommended way to use multiple CPUs is
  "dividing the work up between multiple *processes* rather than multiple
  *threads*"
  <https://docs.python.org/3/faq/library.html> **(direct)**. The paho worker
  still reached 137.7 % of a core because its socket/loop work happens partly in
  C with the GIL released **(interpretation)**.

### Multiple clients / processes

- One paho Python process cannot hold many connections: `select()`'s
  FD_SETSIZE caps it at **~340 connections per process** (3 FDs each × 1024);
  issue #238 is still open, and a commenter's working recipe is "multiprocessing
  instead. Each process [makes] 340 connects"
  <https://github.com/eclipse-paho/paho.mqtt.python/issues/238> **(direct)**.
  (The mosquitto maintainer hit the same wall and scripted multi-process
  subscribers — comment in <https://github.com/eclipse-mosquitto/mosquitto/issues/216>
  **(direct)**.)
- **No first-party source publishes a multi-process/multi-client paho
  throughput curve** **(missing evidence)**. Inference: N paho processes each
  with their own GIL and loop thread scale additively toward the broker/host
  ceiling — bounded above by mosquitto's envelope (§4, ≥78k–100k+ msg/s on
  comparable boxes), not by Python — but each process only earns ~10–12.6k msg/s,
  so 4 processes ≈ 40–50k msg/s of client-side CPU cost.
- The current script at 3 msg/s with per-message waits sits ~3–4 orders of
  magnitude below any of these ceilings; removing the 1 s sleep alone would still
  be limited by the same ~10–12.6k single-process envelope **(inference)**.

## 2. Go paho client (already in go.mod: `github.com/eclipse/paho.mqtt.golang v1.5.1`)

- The API is asynchronous: `Publish()` returns a token, `Wait()`/`<-t.Done()`
  track completion, and the README's own pattern fires error-checking goroutines
  rather than blocking the caller
  <https://github.com/eclipse/paho.mqtt.golang/blob/master/README.md>
  **(direct)**. Parallelism by default: the Go FAQ — `GOMAXPROCS` "default
  value is the number of CPU cores available. Programs with the potential for
  parallel execution should therefore achieve it by default on a multiple-CPU
  machine"
  <https://go.dev/doc/faq> **(direct)** — versus CPython, where "a
  multi-threaded Python program effectively only uses one CPU"
  <https://docs.python.org/3/faq/library.html> **(direct)**.
- There is **no official throughput figure in paho.mqtt.golang's README or docs**
  **(missing evidence)**. Two first-party-adjacent runs exist:
  - A dedicated throughput harness against **mosquitto 2.1.0 on localhost**
    (<https://gist.github.com/gonzalop/bcfea63b0a74745c89543824113b25c6>,
    raw run output; gist intro: run via `gonzalop/mq` `examples/throughput`,
    container with host networking, broker `max_inflight` raised to 1000).
    Its "paho v3" series — almost certainly `eclipse/paho.mqtt.golang` (the MQTT
    3.1.1 Go client), identified from the Go runtime stats every run prints
    (`Alloc/TotalAlloc/NumGC`), the harness context, and Python's measured
    ~12.6k ceiling being two orders of magnitude lower **(interpretation)** —
    reports, single worker, QoS 0, publish → end-to-end delivered:

    | payload | publish rate | delivered end-to-end |
    |---------|-------------:|---------------------:|
    | 20 B    | 214,615/s    | 204,504/s (100 %)    |
    | 128 B   | 198,586/s    | 189,984/s (100 %)    |
    | 1 KiB   | 156,659/s    | 156,557/s (100 %)    |
    | 10 KiB  | 74,794/s     | 73,458/s (100 %)     |

    4 workers reach 292,861/s at 128 B (267,344/s delivered); 20 workers regress
    to ~175–266k/s — more goroutines help ~1.5×, then hurt **(direct +
    interpretation)**. Caveats: machine unidentified in the excerpt, single
    campaign, and the "paho v3/v5" identification is an interpretation.
  - `krylovsk/mqtt-benchmark` — a Go load tool **built on this repo's own
    library** (`require github.com/eclipse/paho.mqtt.golang v1.4.1`,
    <https://github.com/krylovsk/mqtt-benchmark/blob/master/go.mod>) — driven
    unpaced against mosquitto in Docker (WSL2, Ryzen 9 5900X): **100,866 msg/s
    aggregate, QoS 1, 1000 clients, 500 000/500 000 delivered, broker at 103 %
    of one core**
    <https://github.com/johnp789/mqtt-bench> **(direct)**. That is the same
    client stack this repo would use, already demonstrated >100k msg/s through
    mosquitto **(interpretation)**.
- A counter-note: on `eclipse/paho.golang` (the MQTT 5 client, different module)
  a user reports "a single instance of the paho driver can't keep up, we need
  4–5 publisher instances/subscriber"
  <https://github.com/eclipse/paho.golang/issues/328> **(direct, user report,
  workload unspecified)** — Go paho is not infinitely fast; it is fast enough
  that the broker becomes the limit first on every measured box in §4.

## 3. Dedicated MQTT load tools

| tool | first-party rate evidence | notes |
|------|---------------------------|-------|
| `mosquitto_pub` (ships with the broker in this compose stack) | mosquitto maintainer PierreF drives "~30k msgs/s" via a `mosquitto_pub -l` pipe, observing "~1.6M msgs/minute" (≈26.7k/s) on `$SYS` <https://github.com/eclipse-mosquitto/mosquitto/issues/216> **(direct, 2018)** | zero-install sanity load; crude pacing (shell loop) |
| `mqtt-benchmark` (Go, paho.mqtt.golang) | default pacing (`-message-interval 1`) yields only ~676 msg/s total in its own README; with `-message-interval 0` the same tool does **100,866 msg/s** vs mosquitto <https://github.com/krylovsk/mqtt-benchmark> <https://github.com/johnp789/mqtt-bench> **(direct)** | integer-second pacing ⇒ rate granularity = `clients × 1 msg/s`; good for coarse ramps, and `0` for full blast |
| `mqtt-stresser` (Go, inovex) | against **mosquitto 2.0.11**: 500 clients, 50 000 QoS 0 msgs, 100 % published and received; per-client publish throughput median 12 763/s, fastest 229 259/s (aggregate rate not stated by the output) <https://github.com/fhmq/hmq/issues/167> **(direct, user run)** | per-client figures only; tool output label ("Messages/Client: 50000") conflicts with the stated config (100/client) **(contradiction, §5)** |
| `emqtt_bench` (EMQX, Erlang) | EMQX official docs: throughput test shows `rate=99725(msg/sec)` received, stated goal "create `100k/s Qos 0` message throughput" <https://docs.emqx.com/en/emqx/latest/performance/benchmark-emqtt-bench.html> **(direct)** | measured against **EMQX**, not mosquitto; millisecond interval flags give fine rate control |
| `mqttx bench pub` (EMQX, Node) | official docs describe `mqttx bench pub` <https://mqttx.app/docs/cli/get-started>; **no first-party max-rate figure published** (release notes only show a 1 000 msg/s CPU test <https://www.emqx.com/en/blog/mqttx-v-1-9-1-release-notes>) | `-im` interval-ms + `-c` clients ⇒ controllable ramp; headroom unverified |
| k6 `k6/x/mqtt` | Grafana docs call it "an official extension" <https://grafana.com/docs/k6/latest/javascript-api/k6-x-mqtt/>; the repo README warns "experimental … not officially supported yet" <https://github.com/grafana/xk6-mqtt> **(direct, sources disagree — §5)** | no published throughput figures found |

## 4. Mosquitto's localhost throughput envelope (so a broker ceiling is never read as the logger's)

Measured, first-party, in order of freshness:

- **78,269 msgs/s** — broker's own C→C ceiling, `eclipse-mosquitto:2.1.2-alpine`,
  one core, loopback, 256-byte payloads, `$SYS`-verified
  <https://yoch.github.io/mqtt-python-client-bench/> (report header, campaign
  2026-10-05) **(direct)**.
- **100,866 msg/s** sustained QoS 1 (1000 clients, Go tool), mosquitto in Docker
  on WSL2 Ryzen 9 5900X, broker ≈104 % of one core, undated README
  <https://github.com/johnp789/mqtt-bench> **(direct)**.
- **≥204.5k msg/s delivered** (20 B QoS 0, 100 % end-to-end; a third client
  delivered 435,963/s) against **mosquitto 2.1.0** with host networking, one
  core not identified
  <https://gist.github.com/gonzalop/bcfea63b0a74745c89543824113b25c6>
  **(direct, single campaign; `max_inflight` raised to 1000 — affects QoS 1/2
  only, so the QoS 0 rows are unaffected)**.
- Older, stale-but-first-party: mosquitto 1.4.9 did 50 000 msg/s (1 pub, 1 sub,
  QoS 0, 1 KB, single core, 2016)
  <https://github.com/eclipse-mosquitto/mosquitto/issues/216> **(direct)**; the
  same issue's maintainer repro (2018) targets ~30k msg/s via `mosquitto_pub`.
- mosquitto 2.0.11 delivered 50 000/50 000 QoS 0 messages with 500 stresser
  clients (2022) <https://github.com/fhmq/hmq/issues/167> **(direct)**.

**Working envelope (interpretation):** on this class of dev box mosquitto on
loopback sustains **at least ~100k msg/s** (78k on a 2012 i7, 100.9k on a 2020
Ryzen, 200k+ on the gist's machine) — single listener, one core, growing with
CPU generation. The load generator must be able to reach *at least* this class
of rate, otherwise the harness (not the logger, not the broker) is what gets
measured. Precedent for keeping the broker out of the verdict: the harness in §1
measures the broker's C→C ceiling explicitly and flags runs where "Mosquitto was
at its core limit" (`broker_bound`) — this repo's runs should read
`$SYS/broker/load/messages/received/1min` the same way.

### Queue / inflight defaults relevant to a high-rate QoS 0 stream

From the official mosquitto.conf man page (mosquitto.org, page timestamp
2026-02-09) <https://mosquitto.org/man/mosquitto-conf-5.html> **(all direct)**:

- `max_queued_messages` default **1000** — but it applies to **"QoS 1 or 2
  messages … per client"** above those in flight; `0` = no maximum ("not
  recommended").
- `max_queued_bytes` default **0 (no limit)** — likewise **"outgoing QoS 1 and 2
  messages"**.
- `queue_qos0_messages` default **false** — QoS 0 of *disconnected* persistent
  clients is not queued at all.
- `max_inflight_messages` default **20** — **"outgoing QoS 1 or 2 messages"**
  only; `0` = no maximum.
- `max_packet_size` default 2,000,000 bytes since mosquitto 2.1.

Reading **(interpretation)**: none of the queue/inflight limits throttle a
*connected* QoS 0 publish stream — they scope to QoS 1/2 — so for this
harness's QoS 0 load the broker's ceiling is CPU, not `max_queued_messages`.
The limits bite on the QoS > 0 paths: the paho Go README itself warns that
mosquitto's `max_queued_messages`/`max_queued_bytes` defaults "may lead to
messages being silently dropped"
<https://github.com/eclipse/paho.mqtt.golang/blob/master/README.md> **(direct)**.

## 5. Contradictions and cross-checks

- **Python single-client QoS 0 numbers**: 9.5–9.6k msg/s (PR #932's
  `pub_qos_sweep_telemetry`, Mosquitto 2.0.20) vs "11.3k → 11.7k" (harness
  README v1→v2 cross-validation) vs 12,595 (live report `pub_qos0_max`, campaign
  2026-10-05). Same harness family, different scenarios and campaigns — recorded
  as a ~10–12.6k envelope rather than collapsed to one figure.
- **k6 MQTT extension status**: Grafana's docs call `k6/x/mqtt` "an official
  extension"; `grafana/xk6-mqtt`'s README says "experimental … not officially
  supported yet". Sources disagree; neither publishes throughput numbers.
- **`mqtt-stresser` output label** says "Messages / Client: 50000" while the
  stated command is `-num-clients=500 -num-messages=100` (50 000 total). The
  per-client rate figures inherit this ambiguity; only the totals
  (50 000/50 000 delivered) are safe to quote.
- **Mosquitto envelope spread**: ~26.7k msg/s (2018, maintainer's machine),
  50k (2016, v1.4.9), 78.3k (i7-3770, 2026), 100.9k (Ryzen 5900X, WSL2),
  ≥200–436k (gist, mosquitto 2.1.0). Hardware- and era-dependent — recorded as a
  range; only a measurement on this box settles it here.
- **The gist's "paho v3/v5" could be misread as Python paho** ("paho" alone
  usually means the Python client). Reading them as the Eclipse *Go* clients is
  an interpretation, supported by the Go runtime stats each run prints and by
  Python's measured ceiling being ~17× lower; flagged wherever the numbers are
  used.
- **PR #932 was closed the day it was opened** (2026-07-15) after two comments
  urging closure, without merge — its benchmark table is still a measurement of
  stock 2.1.0, but the improvements it describes are not in the released client
  **(interpretation)**.

## Recommendation

**Write a small Go rate-ramp generator on the repo's existing
`github.com/eclipse/paho.mqtt.golang` dependency** (option 2), keeping
`scripts/publish_messages.py` as the low-rate dev trickle. Rate headroom:

- **Go generator: ≥100–200k msg/s QoS 0** demonstrated for this client stack —
  214.6k msg/s single-worker published in the localhost gist (§2) and
  100.9k msg/s aggregate QoS 1 through mosquitto from `mqtt-benchmark`, which
  uses the same library (§2). That exceeds mosquitto's measured localhost
  envelope (78–100k+ msg/s, §4), so the harness cannot become the limiter before
  the broker does, and is ~10–20× the expected broker/logger knee range on this
  box.
- **Extending the Python script instead caps the harness at ~10–12.6k msg/s
  QoS 0** single process (§1) — acceptable **only if** the logger's knee turns
  out ≤ ~6k msg/s (leaving ≥2× headroom). That is knowable only after a first
  ramp; the ticket's premise is that the knee may sit above what the current
  script can prove, so the safe default is the Go generator. Multi-process
  Python (one client per process, ≤340 connections each) can stretch this to
  ~40–50k but is inference-backed only (§1) and burns ~110 µs CPU/msg × N.
- **Existing tools as cross-checks, not the primary**: `mosquitto_pub -l` piped
  from a loop (~27–30k msg/s, maintainer-measured) for a zero-install sanity
  load; `krylovsk/mqtt-benchmark -message-interval 0` (~100.9k msg/s QoS 1 via
  the repo's own client library) for a full-blast ceiling run; `mqttx bench pub`
  or `emqtt_bench` if a tool install is acceptable (100k-class, EMQX first-party
  docs, millisecond pacing) — their mosquitto-specific headroom is unmeasured.

Whichever generator is used: ramp in steps, watch
`$SYS/broker/load/messages/received/1min` so a broker-core ceiling (§4) is
never attributed to the logger, and keep per-message `wait_for_publish` only if
QoS ≥ 1 completion accounting is wanted (it is a documented deadlock hazard
inside callbacks, §1).

## Tradeoffs

- **vs. extending `publish_messages.py`**: zero new code, but the measured
  single-process envelope (~10–12.6k msg/s, ~110 µs CPU/msg) may sit *below* the
  logger's knee — the exact failure the ticket asks to rule out — and multi-process
  scaling has no published measurement behind it.
- **vs. `mqtt-benchmark` as the harness**: it is built on the repo's library and
  proven at 100.9k msg/s, but its pacing is integer-second per client (coarse
  ramps) and its QoS-1-ack-latency-oriented output is not shaped around
  published-vs-persisted attribution for this logger.
- **vs. `mqttx bench pub` / `emqtt_bench`**: finest pacing ergonomics and
  documented 100k-class capability, but against EMQX — mosquitto headroom
  unverified — and adds a Node/Erlang toolchain to a Go repo.
- **Go generator**: full control of ramp, topics, payload mix and rate
  accounting; uses a pinned dependency already in `go.mod` (v1.5.1); one `go run`
  on the dev box. Cost: a new (throwaway-able) tool file plus tests.

## Missing evidence / caveats

- No first-party measurement isolates the cost of one `wait_for_publish()`
  call, nor a multi-process Python paho scaling curve (§1).
- The harness docs do not state whether its paho adapter waits per message;
  ~12.6k msg/s is "the fastest way its own API allows" either way (§1).
- `mqttx bench pub` and k6 `k6/x/mqtt` publish no first-party max-rate figures.
- The gist's "paho v3/v5" client identification and its hardware are
  interpretations/absent; `mqtt-benchmark`'s mosquitto run states no broker
  version; both are single-campaign numbers.
- §4's envelope is machine-dependent by construction; the only number that
  matters for this box is one measured on this box during the ramp.
- Mosquitto's documented defaults do not cover QoS 0 drop behaviour toward a
  *slow connected subscriber*; if the logger falls behind, whether mosquitto
  sheds QoS 0 was not established from the fetched docs.
