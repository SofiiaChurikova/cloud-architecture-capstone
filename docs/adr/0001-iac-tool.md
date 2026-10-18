# ADR-0001: CloudFormation as the IaC tool

**Status:** Accepted

## Context
All infra has to be in code and I need to be able to recreate it from scratch. Everything runs in the AWS Academy sandbox: credentials are temporary, I can't create IAM roles, and the sandbox deletes everything when the session ends. The project is AWS only.

## Options
1. **CloudFormation** - built into AWS, AWS keeps the state for me.
2. **Terraform** - works with any cloud, but needs a state file in S3. The bucket has to be created outside Terraform, and since the sandbox wipes everything after a session, I would have to set it up again every time.
3. **AWS CDK** - code instead of YAML, but `cdk bootstrap` creates IAM roles and the sandbox doesn't allow that. You can skip bootstrap with `cdk synth` + `aws cloudformation deploy`, but then it's just a CloudFormation generator with an extra Node.js setup.

## Decision
CloudFormation with YAML templates.

## Why
- No state file to take care of.
- Change sets show what will change before I apply it, drift detection shows manual changes.
- If a deploy fails, it rolls back by itself.
- I only use AWS, so multi-cloud doesn't matter here.

## Consequences
- (+) I only need the AWS CLI, also for CI/CD in M2.
- (-) YAML is long and not very flexible.
- (-) It only works for AWS.

**Where it lives:** `infra/network.yaml`, `infra/iam-readonly.yaml`
