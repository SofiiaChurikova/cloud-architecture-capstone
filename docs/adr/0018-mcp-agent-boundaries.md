# ADR-0018: What an agent may do through MCP

**Status:** Accepted

## Context
M4 asks to open the store to AI agents over MCP with EXISTS / CREATE / READ / DELETE tools, and to decide what an agent may and may not do and how it is authorised. An agent follows text it reads, so data in the store can contain instructions (prompt injection). Whatever a tool allows, the agent may do by mistake.

Right now the coordinator API has no auth at all, only `ClientCidr` on the security group.

## Options for the transport
1. **MCP over HTTP inside the coordinator** - one more public endpoint, needs its own auth.
2. **Separate MCP server over stdio** - a small program that the MCP client (Claude Desktop) starts on my laptop. It calls the normal coordinator API. Nothing new is open on the network.

## Options for auth
1. **No auth, only `ClientCidr`** - anyone from the allowed IPs can do everything.
2. **One token (`API_TOKEN`) on the coordinator** - every `/tables` request needs `Authorization: Bearer`. The MCP server gets the token from its env.
3. **Separate tokens with roles** - the right way for many users, too much for one user and one agent now.

## Decision
Stdio MCP server (`cmd/mcp`) + one token (option 2).

What the agent **may** do:
- `list_tables`, `exists_item`, `read_item` - reading is safe;
- `create_item` - create one new item in an existing table. It fails if the item exists, so the agent can't overwrite data (an overwrite would be as bad as a delete, and with last write wins it could also lose someone else's update);
- `delete_item` - one key, only with `confirm: true`, and at most 5 deletes per MCP session (`MCP_MAX_DELETES`).

What it **may not** do (not exposed at all):
- create or delete tables - one wrong call deletes everything;
- set `w` / `r` - the agent can't weaken consistency;
- `/internal/*` - replica registration, logs, targets.

Why delete is allowed: M4 asks for DELETE, and deleting one key is something a user may really want from an agent ("remove test user 42"). It's limited to one key and a few per session, and a delete is a new version in the log (tombstone).

`confirm: true` alone is not a human check - the model sets it itself, and a prompt injection can tell it to. The real check is in the MCP client: every tool has annotations (`readOnlyHint` for list/exists/read, `destructiveHint` for delete), and Claude Desktop asks the user before a tool that is not read-only, unless the user turned that off. `confirm` and the limit are a second line, if the client is set to allow everything.

## Consequences
- (+) The agent can't do more than the five tools, whatever text it reads, and the worst it can do is delete 5 keys.
- (+) Nothing new is open on the network.
- (-) The token is the same for the agent, YCSB and me. The coordinator can't tell them apart, the limits live in the MCP server. Roles per token would be the next step.
- (-) Values read from the store go to the agent as they are. If a value contains instructions, the agent sees them. The tools return data as data, and the agent can't delete more than one key at a time even if it is tricked.

- (-) The token goes over plain HTTP to the coordinator's public IP. The sandbox has no TLS setup; `ClientCidr` limits who can connect.
- (-) `create_item` checks "exists" and then writes, so two clients can still race. A conditional write (`If-None-Match`) in the coordinator would close it.

**Where it lives:** `cmd/mcp`, `cmd/coordinator` (`API_TOKEN`)
