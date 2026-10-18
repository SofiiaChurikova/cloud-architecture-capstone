# M1 - Task 6: Quotas that can limit M2

In M2 I need 1 coordinator + 2 shards on EC2 (3 instances, about 6 vCPU), 1 VPC, 4 subnets and maybe 1 public IP.
In M3 every shard gets 3 replicas: 1 + 2 x 3 = **7 instances**. In M5 there is autoscaling and new shards.

## Limits

| Limit | Value | Is it a problem? | Source |
|---|---|---|---|
| **Running instances (sandbox rule)** | **9**, more than that **suspends the lab account** | M2 no (3). M3 close (7). **M5 yes**, autoscaling or a new shard can go over 9 | `evidence/00-sandbox-rules.md` |
| **Instance types (sandbox rule)** | only `t2`/`t3` from `nano` to `medium` | max 2 vCPU / 4 GB per node, no big vertical scaling | `evidence/00-sandbox-rules.md`, `t3.large` denied in `evidence/01-permissions.md` |
| **EBS (sandbox rule)** | up to 35 GB per volume, only `gp2` | no, shards need a few GB | `evidence/00-sandbox-rules.md` |
| **Regions** | other regions blocked by an SCP (tested `eu-central-1`) | no multi-region | `evidence/01-permissions.md` |
| **Services (sandbox rule)** | EC2, Auto Scaling, ELB, CloudFormation, CloudWatch, DynamoDB, Lambda, RDS, S3, SNS, SSM and a few more. **No ECS/Fargate, SQS, Kinesis, MSK, API Gateway** | nodes have to run on EC2, the replication log in M3 will be on S3 or self-hosted | `evidence/00-sandbox-rules.md` |
| On-Demand Standard vCPUs (`L-1216C47A`) | 256 (AWS default is 5) | no, the sandbox rules are stricter (9 x 2 vCPU = 18) | `screenshots/06-quotas-ec2-vcpu.png` |
| VPCs per Region (`L-F678F1CE`) | 5 | no, I use 1 + the default VPC | `evidence/06-quotas.md` |
| EC2-VPC Elastic IPs (`L-0263D0A3`) | 5 | no, max 1 for the coordinator | `evidence/06-quotas.md` |
| IAM | read-only | can't create roles for nodes | `evidence/01-permissions.md` |

## Conclusions

- The real limits come from the sandbox rules, not from Service Quotas. The account quota says 256 vCPU, but I can only run 9 instances up to `medium`.
- **9 instances is the most important limit.** It's ok for M2 and M3, but in M5 autoscaling has to stay under it, otherwise the account gets suspended. So in M5 I'll need a hard `MaxSize` in the Auto Scaling group, or a few replicas on one instance.
- There is no ECS, SQS or Kinesis, so the nodes and the replication log will be on EC2 / S3.
