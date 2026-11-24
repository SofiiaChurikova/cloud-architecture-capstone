# M4 - AI design review

I gave an AI chat my M4 proposal, ADR-0015..0018, the metrics, replica, coordinator, shard and MCP code, the Prometheus config, the alert rules and the monitoring part of the template, and asked it to critique the design. The prompt and the answer are in `ai-review-transcript.md`.

It found 29 points. I checked each one against the code before deciding. The most serious one was a real bug from M3: I reproduced it with a test - two fake followers (`/internal/log?replica=ghost&from=1000000000`) made a `w=3` write return `200` with no real followers.

## My decisions

| # | AI point | Decision | Why | Change |
|---|---|---|---|---|
| 1 | **4** Ack accounting: any name and any `from` count towards W; a follower that comes back with an empty disk keeps its old ack (HIGH) | **Accept** | Reproduced: `w=3` got `200` with two fake followers. | The leader answers `409` when `from - 1` is past the end of its log, and stores the ack as reported, not the max. Tests `TestAckPastTheLogIsRefused`, `TestReplacedFollowerStartsFromZero` |
| 2 | **22, 23** `/internal/*` is on the public port: fake leader, new shard ID, SSRF through Prometheus, text in alert labels (HIGH) | **Accept** | True, and I already put this off in the M2 and M3 reviews. | ADR-0019: `/internal/*` and `/metrics` on port 9080, open only to the shard and monitoring security groups; IDs must match `^[a-z0-9-]{1,32}$`, addresses `http://host:port`. Test `TestInternalIsNotOnThePublicPort` |
| 3 | **1** Fast `503`s go into the latency histogram, so p99 drops during an outage (HIGH) | **Accept** | True: "leader is down" answers in under 1 ms. | `5xx` are counted in `kv_requests_total` but not in the histogram. Test `TestFailedRequestsAreNotInLatency`, ADR-0016 |
| 4 | **8, 9** The availability SLO has no alert; ADR says 5 minutes, rules use 1 minute; no minimum traffic (HIGH) | **Accept** | True. | `HighErrorRate` (> 0.5% `5xx`), all latency and error rules on 5-minute windows and only with > 1 req/s. ADR-0016 says the SLO is per session and the alerts use 5-minute windows. I didn't do multi-window burn rate: with 3-hour sessions a 1-hour window is a third of the session |
| 5 | **19, 20** `confirm=true` is set by the model itself; `create_item` overwrites without any check (HIGH) | **Accept** | True: ADR-0018 claimed `confirm` makes the agent ask the user, it doesn't. The AWS demo confirmed it: Claude Desktop deleted item 100 without any approval window. | Tool annotations (`readOnlyHint`, `destructiveHint`) so Claude Desktop asks me; at most 5 deletes per session; `create_item` only creates (fails if the item exists). ADR-0018 rewritten. Tests `TestCreateNeverOverwrites`, `TestDeleteLimit`, `TestAnnotations` |
| 6 | **16, 17, 18** journald drops lines under load; logs don't say why a request failed or which shard; trace IDs are not checked | **Accept (small part)** | journald's default limit is 10 000 lines / 30 s, YCSB is above it. | `LogRateLimitIntervalSec=0` in the units; log lines get `shard_id`, `replicas`, `version`, and `5xx` are at error level with the reason; trace IDs must be 1-64 `[A-Za-z0-9_-]`. Test `TestBadTraceIDIsReplaced` |
| 7 | **25** Numbers above 2^53 change on read -> write; a `503` after the leader wrote looks like a failure | **Accept** | Both true and cheap. | `UseNumber` in the MCP server; `503` with a version says "the leader already has it as version X, check before retrying". Test `TestNotConfirmedWrite` |
| 8 | **14** Detection time not written down; no `scrape_timeout` | **Accept** | Control question 3 asks exactly this. | `scrape_timeout: 3s`, `ShardMissingReplicas` `for: 15s`, the budget is in ADR-0016 (6 s for routing, ~30-50 s for alerts) |
| 9 | **27** ADR-0017 says a replica can get entries out of order; the code doesn't allow it | **Accept (docs)** | True: `AppendAt` keeps strict offset order. | ADR-0017 context rewritten: LWW matters for reads with R > 1 and for M5 key moves |
| 10 | **10** Alerts go nowhere, no Alertmanager (HIGH) | **Refute for M4** | M4 part 1 asks to "set up alerting for critical events", and the alerts fire and show in Prometheus and on the Grafana dashboard. Sending them to a channel with context is part 4.1 "automated incident response", which is optional and which I skip. The M5 ops agent can read `GET /api/v1/alerts` from Prometheus. | ADR-0016: "alerts are not sent anywhere" |
| 11 | **28** 8 of 9 instances leaves nothing for M5 (HIGH) | **Refute for M4** | Already decided: in M5 several replicas (of different shards) run on one instance, so a new shard is processes, not machines. That's M5 work, not something M4 asks for. | - |
| 12 | **11** When the coordinator dies, alerts disappear | **Refute** | The coordinator is a static Prometheus target, so `up{job="coordinator"} == 0` fires `NodeDown`; I tested it locally. Only the death of Prometheus itself is not seen, that needs something outside (a dead man's switch), which needs a receiver - see 10. | - |
| 13 | **24** Auth is off when `API_TOKEN` is empty; the token goes over plain HTTP | **Refute (partly)** | Empty token is on purpose for local runs and tests; in AWS the pipeline always passes the secret. There is no TLS setup in the sandbox, `ClientCidr` limits who can connect. Written in ADR-0018 as a minus. | ADR-0018 |
| 14 | **21** One shared token, no roles, no audit | **Refute for M4** | M4 asks to decide how the agent is authorised, and the decision and its limit are in ADR-0018. Roles per token are the next step, not needed for one user and one agent. | - |

## Notes for later

- **2, 3, 7, 13** Per-shard load in the coordinator metrics, saturation (in-flight requests, memory, node_exporter), finer buckets, scaling target below the SLO - this is what M5 autoscaling needs.
- **5, 6** Lag for a follower that never connected, heartbeat that says "healthy" and not just "alive" - M5, together with the replica list per shard.
- **12, 15** `kv_replicas_expected` instead of the fixed 3, deregistering removed replicas - M5, when replica counts change.
- **16** Central logs (Loki) - not in M4; this is my answer to control question 8 about what is missing.
- **18** Tracing the replication itself (leader write -> follower apply) - logs can be joined by version for now.
- **26** Hybrid logical clock when keys move between leaders - M5 (in ADR-0017).
- **29** Checksums for the downloads, non-root services, Prometheus data on its own volume - small, but the sandbox wipes everything after a session anyway.
