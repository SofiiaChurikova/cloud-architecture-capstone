# ADR-0008: CI/CD with GitHub Actions

**Status:** Accepted

## Context
The pipeline has to deploy infra and app. Sandbox credentials work only 3 hours, and IAM is read-only.

## Options
1. **GitHub Actions** - the repo is already on GitHub, credentials go into repo secrets.
2. **AWS CodePipeline / CodeBuild** - not in the list of allowed sandbox services.

## Decision
GitHub Actions: build on every push, then deploy with the sandbox credentials from secrets.

## Why
- CodePipeline / CodeBuild aren't available in the sandbox.
- Nothing extra to set up.

## Consequences
- (+) Build always runs.
- (-) Deploy works only during a sandbox session. At the start of a session I update the 3 secrets with `gh secret set`.

**Where it lives:** `.github/workflows/`
