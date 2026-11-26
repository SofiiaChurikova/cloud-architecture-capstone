# Spec Delta

## Purpose

Every service says if it is up and which build it runs.

## ADDED Requirements

### Requirement: Health and version
The coordinator and every replica SHALL answer `GET /health` and `GET /version`. `/version` SHALL return the git commit the binary was built from (`dev` for a local build).

#### Scenario: Version after a deploy
- **WHEN** I call `/version` on the coordinator after a deploy
- **THEN** it returns the commit of that pipeline run
