# M1 - Task 8: Recreating the stack

I deleted the `capstone-network` stack and created it again only from `infra/network.yaml`.

## Commands

```bash
aws cloudformation delete-stack --stack-name capstone-network
aws cloudformation wait stack-delete-complete --stack-name capstone-network
aws cloudformation deploy --stack-name capstone-network --template-file infra/network.yaml
```

Full log with times: `evidence/08-recreate.md`.

## How long it took (2026-10-18)

| Step | Start | End | Time |
|---|---|---|---|
| Delete | 23:08:01 | 23:08:36 | **35 s** |
| Create | 23:08:36 | 23:09:45 | **69 s** |
| **Total** | | | **1 min 44 s** |

## What broke on the first try

Nothing, it worked the first time. After that the stack had the same exports again
(`capstone-network-VpcId`, `capstone-network-PublicSubnetIds`, `capstone-network-PrivateSubnetIds`).

What could have broken and why it didn't:

- **An export that another stack uses can't be deleted.** No other stack uses the network yet. From M2 I'll have to delete the app stack first (ADR-0002).
- **Something created by hand inside the VPC** (an instance or a security group from the console) would block deleting the VPC. I didn't create anything outside CloudFormation.
- **The route to the IGW can't be created before the IGW is attached.** The template has `DependsOn: GatewayAttachment` for that.

The IAM stack `capstone-iam` wasn't part of this test, because it can't be created in the sandbox at all (`evidence/03-iam-template.md`).

## What it shows

- The network exists only as code, two commands bring it back in less than 2 minutes.
- The sandbox deletes everything after each session anyway, so I rebuild the stack from the template every time I work.
