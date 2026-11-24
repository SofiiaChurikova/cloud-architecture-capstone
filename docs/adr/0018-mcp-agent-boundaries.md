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
- `create_item` - create or replace one item in an existing table;
- `delete_item` - one key, only with `confirm: true`.

What it **may not** do (not exposed at all):
- create or delete tables - one wrong call deletes everything;
- set `w` / `r` - the agent can't weaken consistency;
- `/internal/*` - replica registration, logs, targets.

Why delete is allowed: M4 asks for DELETE, and deleting one key is something a user may really want from an agent ("remove test user 42"). It's limited to one key, a delete is a new version in the log (tombstone), and `confirm: true` makes the agent ask the user first.

## Consequences
- (+) The agent can't do more than the five tools, whatever text it reads.
- (+) Nothing new is open on the network.
- (-) The token is the same for the agent, YCSB and me. The coordinator can't tell them apart, the limits live in the MCP server. Roles per token would be the next step.
- (-) Values read from the store go to the agent as they are. If a value contains instructions, the agent sees them. The tools return data as data, and the agent can't delete more than one key at a time even if it is tricked.

**Where it lives:** `cmd/mcp`, `cmd/coordinator` (`API_TOKEN`)
