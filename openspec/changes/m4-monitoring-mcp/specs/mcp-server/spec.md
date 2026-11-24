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
The MCP server SHALL NOT expose deleting or creating tables, the `w` / `r` parameters or `/internal/*`. `delete_item` SHALL delete one key and only with `confirm: true`.

#### Scenario: Delete without confirm
- **WHEN** the agent calls `delete_item` without `confirm: true`
- **THEN** nothing is deleted and the tool says it needs confirmation

### Requirement: Token
When the coordinator has `API_TOKEN` set, every `/tables` request SHALL need `Authorization: Bearer <token>`, otherwise `401`. The MCP server SHALL send the token from its environment (`KV_TOKEN`).

#### Scenario: No token
- **WHEN** a request without the token reaches a coordinator with `API_TOKEN`
- **THEN** it gets `401`
