# ADR-0019: Internal API on its own port

**Status:** Accepted (after the M4 AI review)

## Context
Since M2 the coordinator serves `/internal/shards` on the public port 8080, together with the client API. The M2 and M3 reviews both said it, and I put it off to M4. The M4 review showed why it can't wait: anyone who can reach 8080 can
- register a fake leader for a shard and get all its writes;
- add a new shard ID, which moves keys on the ring, so data looks deleted;
- register any address, and Prometheus (HTTP service discovery) will call it;
- put any text into a replica name, which then shows up in alert texts that an agent may read (M5).

The MCP limits (ADR-0018) don't help here, they only bind the MCP program, not the network. `API_TOKEN` covers only `/tables`.

## Options
1. **Token on `/internal/*` too** - every replica and Prometheus need the token, the port stays open to the internet.
2. **Second listener for `/internal/*` and `/metrics`** - the security group lets only the replicas and the monitoring instance reach it.
3. **Both** - safer, but one more secret on every node.

## Decision
Option 2, plus input checks:
- the coordinator listens on 8080 (client API: `/tables`, `/health`) and 9080 (`/internal/*`, `/metrics`, `/health`);
- 9080 is open only to the shard and monitoring security groups;
- replica and shard IDs must match `^[a-z0-9-]{1,32}$`, the address must be `http://host:port`.

## Consequences
- (+) From the internet nobody can register replicas or read the topology.
- (+) Bad names can't reach alert texts.
- (-) Inside the VPC there is still no auth: a compromised replica could register as something else. A cluster token would be the next step.
- (-) `curl $URL/internal/shards` from my laptop doesn't work any more. Grafana and `/health` show the cluster state instead.

**Where it lives:** `cmd/coordinator` (`internalRoutes`, `validID`, `validAddr`), `infra/app.yaml` (9080 in the security groups), `monitoring/prometheus.yml`
