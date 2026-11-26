# ADR-0022: What the ops agent may do

**Status:** Accepted

## Context
M5 asks for an agent that reads alerts and metrics and proposes an action - rebalance, scale up, scale down - with reasons. Letting it execute the proposal behind a human approval is a bonus, which I skip.

The agent reads data that comes from the cluster. Since M4 replica names are checked (ADR-0019), but an agent that can change the cluster is still the riskiest part: a wrong scale down deletes 3 instances.

## Options
1. **Agent with read-only tools** - it looks and suggests, a human (or the autoscaler) acts.
2. **Agent with tools to scale and drain** - closes the loop, but needs approvals and limits, and it is the bonus part.
3. **Program that calls an LLM API on every alert** - runs on its own, but needs an API key and money, and the same question of what it may do.

## Decision
Option 1: `cmd/opsmcp`, an MCP server over stdio for Claude Desktop, with three read-only tools:
- `get_alerts` - firing and pending alerts from Prometheus: name, state, since, labels; no free text from annotations;
- `query_metric` - one PromQL query to Prometheus (the Prometheus query API can't change anything), with a 5 s timeout;
- `get_cluster` - coordinator `/health` with the move state. It needs no token.

Plus instructions (`docs/m5/ops-agent.md`): answer with one action (scale up / scale down / rebalance / do nothing), the numbers it is based on, the risk, and what to check after.

What the agent **can't** do: change `ShardCount`, drain or add shards, write or delete data, reach the AWS API. `opsmcp` has no credentials for any of it - not even the API token.

Changed after the AI review: "read-only" depends on the client too. Claude Desktop puts every MCP server it has into one chat, and my M4 `kvstore` server has `create_item` / `delete_item`. So for the ops agent I turn the `kvstore` server off in Claude Desktop (or remove it from the config) and keep only `kvstore-ops`. "Rebalance" here means: a move is stuck or the ring doesn't match `ShardCount`, and a human runs the drain again or fixes the stack.

## Consequences
- (+) Whatever the agent reads or is told, the worst it can do is give bad advice.
- (+) No API keys, no cost, same setup as the M4 MCP server.
- (-) It runs when I ask, not by itself on every alert.
- (-) The autoscaler and the agent can disagree; the autoscaler acts on its fixed rules, the agent only explains.

**Where it lives:** `cmd/opsmcp`, `docs/m5/ops-agent.md`
