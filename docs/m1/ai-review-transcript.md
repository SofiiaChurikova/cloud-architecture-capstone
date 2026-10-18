# AI design review - transcript

## Prompt

```text
You are a senior AWS solutions architect reviewing Milestone 1 of a student capstone project.

Project: a "light" DynamoDB-like key-value store, built over 5 milestones:
M1 infrastructure, M2 sharding (coordinator + consistent hashing, CI/CD),
M3 replication (3 replicas per shard, quorums), M4 monitoring + MCP server,
M5 autoscaling and shard rebalancing.

Environment: AWS Academy sandbox. IAM roles and policies cannot be created,
only us-east-1/us-west-2, max 9 instances, limited credit budget.

Milestone 1 includes: a VPC with public and private subnets (CloudFormation),
a read-only IAM policy and role delivered as a template only, service quotas,
a budget for M2, and proving the stack can be recreated from code.

Attached: the OpenSpec proposal and specs, 3 ADRs, the CloudFormation templates.

Please critique the design: network layout, stack separation, CIDR choice,
choice of IaC tool, the read-only IAM policy, and the budget.

For each issue give:
1. severity (high / medium / low),
2. why it matters, especially for milestones M2-M5,
3. a concrete alternative.

Be critical and specific. Do not just praise.
```

## Answer

**Overall:** four high-severity problems - private subnets have no egress, two AZs can't keep a 3-replica quorum, the budget doesn't fit the 9-instance cap, and CI/CD has no credentials plan. Solid: network-vs-app stack split, CloudFormation for this sandbox, recording the IAM refusal as evidence, timing the rebuild. The reviewer did not see the budget/quota reports, only the proposal.

### 1. Network layout
- **1.1 Private subnets have no egress and no VPC endpoints - HIGH.** Nodes can't install packages, pull images, reach SSM or send CloudWatch metrics; cfn-signal would hang. Alternatives: free S3 gateway endpoint now; NAT instance (~$3-4/month, uses 1 of 9 instances); avoid NAT Gateway (~$33/month) and many interface endpoints (~$7.30/month per AZ each); or nodes in public subnets with strict SGs, documented in an ADR. Rewrite the spec scenario "only the local route" as "no route to an Internet Gateway".
- **1.2 Two AZs can't keep a 3-replica quorum through an AZ failure - HIGH (M3).** One AZ holds 2 of 3 replicas; losing it loses the majority. Create 3 AZs now (subnets are free) or state in ADR-0003 that M3 survives a node failure but not an AZ failure.
- **1.3 AZs picked by index from `!GetAZs` - MEDIUM.** Some us-east-1 AZs lack instance types; make AZs parameters and check `describe-instance-type-offerings`.
- **1.4 No security group design or flow logs - LOW-MEDIUM.** Define SG-to-SG model in a shared stack; VPC Flow Logs to S3.

### 2. Stack separation
- **2.1 Comma-joined exports with `ImportValue` lock the network - MEDIUM-HIGH.** An imported export can't change; use SSM Parameter Store, or export each subnet separately and only add exports.
- **2.2 Two layers aren't enough once data is stateful - MEDIUM (M3/M5).** Split into network / platform / coordinator / storage, with `DeletionPolicy: Retain` on data volumes; store ring membership by private IP.
- **2.3 Deletion order is a manual step - LOW.** Use one script / Makefile (`make up` / `make down`).

### 3. CIDR scheme
- **3.1 The /20 private subnets are justified by growth that the 9-instance cap rules out - LOW.** Keep sizes, rewrite the reasoning, reserve a third AZ and `10.21.0.0/16` for us-west-2.
- **3.2 The peering argument is weak - LOW.** Default VPC is `172.31.0.0/16` and nothing is peered.

### 4. IaC tool (ADR-0001)
- **4.1 The decision is fine, but two reasons are wrong - LOW.** Terraform >= 1.10 locks with `use_lockfile` (no DynamoDB table); CDK can avoid bootstrap with `CliCredentialsStackSynthesizer` or `cdk synth` + `aws cloudformation deploy`. Base the ADR on the real reasons.
- **4.2 `validate-template` is the only check - MEDIUM.** Add `cfn-lint` and `cfn-guard` as M2 pipeline gates; use change sets for the storage stack.
- **4.3 "Only the AWS CLI is needed for CI/CD" skips the credentials problem - HIGH (M2).** Sandbox credentials expire; no OIDC/role creation. Options: CodeBuild/CodePipeline with an existing role, or a self-hosted runner on EC2.

### 5. Read-only IAM policy
- **5.1 Missing read access for ELB, Auto Scaling, log contents, templates, quotas, budgets, tags - MEDIUM.** Start from AWS-managed `ViewOnlyAccess` plus a small inline policy.
- **5.2 The spec promises "the project's resources" but the policy uses `Resource: "*"` - LOW-MEDIUM.** Describe/List calls don't support resource-level scoping; fix the wording; no secrets in UserData.
- **5.3 The trust policy doesn't enforce "exactly one principal" - MEDIUM.** `:root` trusts the whole account; add `AllowedPattern`, `MaxSessionDuration`, `sts:ExternalId`; drop fixed names.
- **5.4 Spec scenarios can't be tested the planned way - MEDIUM.** Use `simulate-custom-policy`, `accessanalyzer validate-policy` or Parliament; mark the trust scenario as verified by review.
- **5.5 The policy is decorative while the M4 MCP server would run with sandbox credentials - HIGH (M4).** Enforce read-only in the MCP server with an API allowlist.

### 6. Budget and capacity
- **6.1 Only M2 is budgeted, but the 9-instance cap is used up by M3/M4 - HIGH.** Make a capacity table M1-M5; run several replicas per instance; scale a fixed pool.
- **6.2 Common cost drivers likely missing - MEDIUM.** Public IPv4, ALB, cross-AZ traffic, EBS while stopped, CloudWatch; budget in session-hours; tag everything; keep ~30% reserve; automate teardown.

### 7. Cross-cutting
- **7.1 The LabRole analysis has no spec - MEDIUM.** Add a `sandbox-constraints` capability with a tested allow/deny matrix.
- **7.2 The recreatability test proves too little - MEDIUM.** Rebuild network + platform + minimal app together, also in us-west-2, run drift detection, use one script.

**Reviewer's top 3 before M2:** 1.1 (egress), 1.2 (3 AZs), 6.1 (capacity plan).
