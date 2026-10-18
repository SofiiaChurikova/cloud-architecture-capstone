# Spec Delta

## Purpose

Ensures the base infrastructure exists only as code: it can be fully removed and rebuilt from the repository without manual steps, and the time this takes is known.

## ADDED Requirements

### Requirement: Infrastructure is rebuilt from code only
The network stack SHALL be deleted and recreated using only the templates in the repository, with no manual changes in the console.

#### Scenario: Full teardown and rebuild
- **WHEN** the network stack is deleted and deployed again from `infra/network.yaml`
- **THEN** the stack reaches CREATE_COMPLETE
- **AND** it exports the same names as before

### Requirement: Rebuild time is measured
The time to delete and to recreate the stack SHALL be measured and recorded.

#### Scenario: Timing is recorded
- **WHEN** the stack is recreated
- **THEN** the delete time, create time and total time are written to `docs/m1/recreate.md`

### Requirement: Problems on the first attempt are recorded
Any error during the first rebuild SHALL be recorded together with its cause and fix.

#### Scenario: First attempt fails
- **WHEN** the first rebuild attempt fails or needs manual action
- **THEN** the error, its cause and the fix are written to `docs/m1/recreate.md`
