# ADR-0004: REST + OpenAPI

**Status:** Accepted

## Context
The store needs an API for tables and for CREATE / READ / EXISTS / DELETE. The task lets me pick OpenAPI REST, GraphQL or gRPC.

## Options
1. **REST + OpenAPI** - normal HTTP and JSON, easy to test with curl.
2. **gRPC** - faster, but can't test it with curl and needs generated clients.
3. **GraphQL** - more for flexible queries, too much for simple key-value.

## Decision
REST with JSON, described in `api/openapi.yaml`.

## Why
- Each operation is just an HTTP method: `PUT` = create, `GET` = read, `HEAD` = exists, `DELETE` = delete.
- Easy to test by hand and to call from YCSB.

## Consequences
- (+) Simple to build and debug.
- (-) Slower than gRPC.

**Where it lives:** `api/openapi.yaml`, handlers in `cmd/coordinator/main.go` and `internal/store/store.go`
