# Spec Delta

## Purpose

One request can be followed through the coordinator and the replicas it touched.

## ADDED Requirements

### Requirement: Trace ID
The coordinator SHALL take `X-Trace-Id` from the request (if it is 1-64 letters, digits, `-` or `_`) or create a new one, SHALL send it to every replica it calls and SHALL return it in the response.

#### Scenario: Trace ID in the answer
- **WHEN** a client sends a request without `X-Trace-Id`
- **THEN** the response has a new `X-Trace-Id` and the replicas got the same value

### Requirement: JSON logs
The coordinator and replicas SHALL write one JSON log line per request with the trace ID, method, path, status and duration, plus the shard, replicas and version when known. Requests that end with `5xx` SHALL be logged at error level with the reason.

#### Scenario: Follow a request
- **WHEN** I search the logs of the coordinator and the replicas for one trace ID
- **THEN** I find the coordinator line and the lines of every replica that served the request
