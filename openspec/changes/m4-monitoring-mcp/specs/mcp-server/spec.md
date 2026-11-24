# Spec Delta

## Purpose

An AI agent can use the store through MCP, only within clear limits.

## ADDED Requirements

### Requirement: MCP tools
`cmd/mcp` SHALL be an MCP server over stdio with the tools `exists_item`, `read_item`, `create_item`, `delete_item` and `list_tables`. Every tool SHALL have an input schema, and errors from the store SHALL come back as tool errors with a clear message.

#### Scenario: Read a missing key
- **WHEN** the agent calls `read_item` for a key that doesn't exist
- **THEN** it gets a tool error "item not found", not a crash

### Requirement: Limits for the agent
The MCP server SHALL NOT expose deleting or creating tables, overwriting items, the `w` / `r` parameters or `/internal/*`. `create_item` SHALL fail if the item exists. `delete_item` SHALL delete one key, only with `confirm: true`, and at most `MCP_MAX_DELETES` (default 5) times per session. Tools SHALL carry annotations: read-only for list/exists/read, destructive for delete.

#### Scenario: Create an existing item
- **WHEN** the agent calls `create_item` for a key that exists
- **THEN** nothing changes and the tool says the item already exists

#### Scenario: Delete without confirm
- **WHEN** the agent calls `delete_item` without `confirm: true`
- **THEN** nothing is deleted and the tool says it needs confirmation

### Requirement: Token
When the coordinator has `API_TOKEN` set, every `/tables` request SHALL need `Authorization: Bearer <token>`, otherwise `401`. The MCP server SHALL send the token from its environment (`KV_TOKEN`).

#### Scenario: No token
- **WHEN** a request without the token reaches a coordinator with `API_TOKEN`
- **THEN** it gets `401`
