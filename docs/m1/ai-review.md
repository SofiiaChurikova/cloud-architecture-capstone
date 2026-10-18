# M1 - AI design review

I gave an AI chat my OpenSpec proposal and specs, the three ADRs and both templates, and asked it to critique the design like a senior AWS architect. The prompt and the answer are in `ai-review-transcript.md`.

It found 4 high-severity problems (private subnets with no internet, 2 AZs vs. a 3-replica quorum, the 9-instance limit, credentials for CI/CD). It agreed with the separate stacks, CloudFormation, saving the IAM error as evidence and timing the rebuild.

## My decisions

| # | AI point | Decision | Why | Change |
|---|---|---|---|---|
| 1 | **1.1** Private subnets have no way out to the internet and no VPC endpoints (HIGH) | **Refute for M1**, come back to it in M2 | The task itself says private subnets are "for internal access only", and nothing runs there yet. A NAT gateway costs about $33/month, more than my whole $20 budget in less than 3 weeks (`budget.md`). In M2, when nodes need packages and logs, I'll pick in a separate ADR between a free S3 gateway endpoint, a NAT instance, or public subnets with strict security groups. | nothing in M1 |
| 2 | **4.1** Two reasons in ADR-0001 are wrong: Terraform 1.10+ can lock state in S3 without DynamoDB, and CDK can work without bootstrap (LOW) | **Accept** | Both are true. I still keep CloudFormation, but now the ADR has the real reasons: the sandbox wipes everything after each session, so Terraform state would need to be set up again every time. | ADR-0001 updated |
| 3 | **1.2** 2 AZs can't keep a 3-replica quorum if one AZ goes down (HIGH, M3) | **Accept, but later in M3** | True: with 3 replicas in 2 AZs one AZ always has 2 of them, so losing it loses the majority. But this only matters when replication exists (M3). Subnets are free and the ranges for a third AZ are already free, so I can add it later without changing addresses. | ADR-0003: added to consequences |

## Notes for later

- **6.1** The 9-instance limit will be a problem in M5, it's in `quotas.md`. M5 needs a hard Auto Scaling `MaxSize` or a few replicas per instance.
- **4.3** CI/CD credentials: sandbox credentials expire every session, I'll decide this in M2.
- **2.1** Exports that are already used by another stack can't be changed, I'll check this when the app stack starts using them in M2.
