# M4 demo in AWS

Run on 2026-11-24 against the stack deployed by the pipeline from `main` (8 x t3.micro, us-east-1). Coordinator `http://98.91.176.169:8080`, monitoring `100.54.40.98` (IPs change every session). The token is replaced with `$TOKEN`.

## 1. Cluster, token, trace ID

```
$ curl $URL/health
{"n":3,"r":2,"replicas":6,"shards":2,"status":"ok","w":2}

$ curl -o /dev/null -w "%{http_code}" $URL/tables
401

$ curl -H "Authorization: Bearer $TOKEN" -X PUT $URL/tables/users -d '{"partition_key":"id"}'
{"name":"users","partition_key":"id"}

$ curl -i -H "Authorization: Bearer $TOKEN" -X PUT $URL/tables/users/items/42 -d '{"id":"42","name":"Ann"}'
HTTP/1.1 200 OK
X-Shard-Id: shard-1
X-Trace-Id: ea1f1f2c77e8c043
X-Version: 1795536195903612

$ curl -o /dev/null -w "%{http_code}" $URL/internal/shards
404
```

- Without the token `/tables` gives `401`.
- The version is the leader time in microseconds (ADR-0017).
- `/internal/*` is not on the public port (ADR-0019).

## 2. Prometheus and Grafana

Prometheus scrapes the coordinator on the internal port 9080 and the 6 replicas it got from `/internal/targets` - `screenshots/01-prometheus-targets.png`.

The Grafana dashboard after a few minutes of writes and reads from my laptop - `screenshots/02-dashboard.png`, `screenshots/03-dashboard-shards.png`: 3/3 alive replicas per shard, 7/7 targets, throughput, p50/p95/p99 for the coordinator and the shards, keys per shard.

## 3. A follower goes down

Writes running in a second terminal, then:

```
$ aws ec2 stop-instances --instance-ids i-02fcd6f6d6c84d0aa     # shard-1-b
"CurrentState": "stopping", "PreviousState": "running"
```

- First `NodeDown`, `ShardMissingReplicas` and `ReplicationLag` are pending (waiting for their `for:`) - `screenshots/04-alerts-pending.png`.
- Then all three are firing - `screenshots/05-alerts-firing.png`.
- The dashboard shows shard-1 with 2 alive replicas, 6 of 7 targets up, and the alerts list - `screenshots/06-dashboard-node-down.png`, `screenshots/07-dashboard-alerts-firing.png`.
- `HighErrorRate` stays inactive: writes use W=2, so the leader and shard-1-c are enough and clients get no errors.

## 4. The follower comes back

```
$ aws ec2 start-instances --instance-ids i-02fcd6f6d6c84d0aa
"CurrentState": "pending", "PreviousState": "stopped"
```

After ~2 minutes:
- all alerts are inactive again - `screenshots/08-alerts-resolved.png`;
- shard-1 has 3 alive replicas and 7/7 targets are up - `screenshots/09-dashboard-recovered.png`;
- the lag of shard-1-b grew to almost 2 minutes while it was stopped and dropped to 0 when it caught up from its log - `screenshots/10-lag-back-to-zero.png`.

## 5. MCP from Claude Desktop

`bin/mcp` (built for my Intel Mac) in `claude_desktop_config.json` with `KV_URL` = the coordinator in AWS and `KV_TOKEN`. First a check from the terminal:

```
$ printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_tables","arguments":{}}}' | KV_URL=$URL KV_TOKEN=$TOKEN ./bin/mcp
{"jsonrpc":"2.0","id":1,"result":{"content":[{"text":"{\"tables\":[{\"name\":\"users\",\"partition_key\":\"id\"}]}","type":"text"}],"structuredContent":{"tables":[{"name":"users","partition_key":"id"}]}}}
```

Then a new chat in Claude Desktop, one request at a time:

| I asked | Tool | Result |
|---|---|---|
| Which tables are in kvstore? | `list_tables` | `users`, partition key `id` |
| Read item 42 from users | `read_item` | `{"id":"42","name":"Ann"}`, version 1795536195903612 |
| Create item 100: id 100, name Sofiia | `create_item` | created, version 1795537335794074 |
| Create item 100 again with name Test | `create_item` | **failed**: the item exists, `create_item` doesn't overwrite. The agent offered to delete and create again, or use another key, and asked me |
| Delete item 100 | `delete_item` | deleted. Claude Desktop showed no approval window: the model set `confirm: true` itself because I asked to delete (ADR-0018). The limit of 5 deletes per session is what stops it from deleting more |

Screenshots: `screenshots/11-mcp-list-and-read.png`, `screenshots/12-mcp-create.png`, `screenshots/13-mcp-create-again-fails.png`, `screenshots/14-mcp-delete.png`.
