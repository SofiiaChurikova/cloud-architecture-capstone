# MCP server

`cmd/mcp` is an MCP server over stdio (ADR-0018). The MCP client starts it on my laptop, and it calls the coordinator API. Nothing new is open on the network.

## Tools

| Tool | What it does | Store call |
|---|---|---|
| `list_tables` | list tables | `GET /tables` |
| `exists_item` | does an item exist | `HEAD /tables/{t}/items/{k}` |
| `read_item` | read one item and its version | `GET /tables/{t}/items/{k}` |
| `create_item` | create or replace one item | `PUT /tables/{t}/items/{k}` |
| `delete_item` | delete one item, only with `confirm: true` | `DELETE /tables/{t}/items/{k}` |

Not exposed on purpose: creating or deleting tables, `w` / `r`, `/internal/*`.

Errors (missing table or item, wrong token, store down, delete without confirm) come back as tool errors with a short message, so the agent can tell the user what happened.

## Build

```bash
GOOS=darwin GOARCH=amd64 go build -o bin/mcp ./cmd/mcp     # Intel Mac
GOOS=darwin GOARCH=arm64 go build -o bin/mcp ./cmd/mcp     # Apple Silicon
```

The pipeline also builds both and keeps them in the `binaries` artifact.

## Claude Desktop

Settings - Developer - Edit Config, then add to `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "kvstore": {
      "command": "/Users/macbook/Desktop/cloud-architecture-capstone/bin/mcp",
      "env": {
        "KV_URL": "http://<coordinator-ip>:8080",
        "KV_TOKEN": "<the API_TOKEN secret>"
      }
    }
  }
}
```

Restart Claude Desktop, the `kvstore` tools show up in the tools menu.

## Protocol

MCP over stdio is JSON-RPC 2.0, one message per line. The server answers `initialize`, `ping`, `tools/list` and `tools/call`, and ignores notifications. I wrote it without the Go SDK: it's 4 methods, and the SDK pulls in packages from `golang.org/x` that I couldn't download in my build environment. I checked it with the official MCP client (Python `mcp` package) against a local cluster, see `docs/m4/evidence/local-run.md`.
