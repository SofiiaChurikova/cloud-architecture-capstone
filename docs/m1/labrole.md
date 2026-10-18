# M1 - Task 1: LabRole and sandbox permissions

## There is no LabRole here

The task talks about `LabRole`, but the Cloud Architecting sandbox doesn't have it (it exists in Learner Lab).
`aws iam list-roles` shows service roles, roles of the lab platform (`vocareum`, `vocareum-eventbridge`), some ready roles (`lambda-run-role`, `myBasicLambdaRole`, `myS3Role`, `EMR_*`) and **`voclabs`**.

`aws sts get-caller-identity` shows that I work as:
`arn:aws:sts::496270268046:assumed-role/voclabs/user5166800=...`

So I checked **`voclabs`** instead of LabRole.

## Policies on voclabs

| Policy | Type | Where I saw it |
|---|---|---|
| `ReadOnlyAccess` | AWS managed | `list-attached-role-policies` |
| `Pvoclabs` | inline | only in the policy simulator output (`role_voclabs_Pvoclabs`) |

I couldn't read the policies themselves. When I open the role in the console I get `iam:GetRole ... an identity-based policy explicitly denies the action` (`screenshots/01-voclabs-getrole-denied.png`).

## What is allowed and what is not

| Action | Result | How I checked |
|---|---|---|
| `iam:ListRoles` | allowed | real call |
| `iam:GetRole` on `voclabs` | denied, explicit deny | console |
| `iam:CreateUser` | denied, implicit ("no identity-based policy allows") | real call |
| `iam:CreateRole` | denied, implicit | policy simulator |
| `ec2:CreateVpc` | allowed (by `Pvoclabs`) | simulator + `--dry-run` |
| `ec2:RunInstances` `t3.micro` | allowed | `--dry-run` |
| `ec2:RunInstances` `t3.large` | denied, explicit deny | `--dry-run` |
| any EC2 call in `eu-central-1` | denied, explicit deny in an SCP | real call |

All outputs are in `evidence/01-permissions.md`. The official sandbox rules are in `evidence/00-sandbox-rules.md` (instances up to `medium`, max 9 instances, EBS up to 35 GB gp2, IAM read-only).

## How I found out

1. `aws sts get-caller-identity` - who I am.
2. `aws iam list-roles` and `list-attached-role-policies` - which roles and policies exist. Reading the role in the console didn't work.
3. Policy simulator (`simulate-principal-policy`) for `ec2:CreateVpc` and `iam:CreateRole`.
4. Real calls: EC2 with `--dry-run`, IAM create calls, a call to another region.
5. The sandbox description page.

## Three kinds of deny I saw

1. **Implicit deny** - nothing allows the action (`iam:CreateUser`, `iam:CreateRole`).
2. **Explicit deny in the role policy** - it wins over any allow. `ReadOnlyAccess` normally allows `iam:GetRole`, but an explicit deny on its own role blocks it. Same for big instance types.
3. **Explicit deny in an SCP** - a rule for the whole account from AWS Organizations, here it blocks other regions. The simulator doesn't show it, that's why real calls are needed too.

## What this means for the project

- IAM is read-only, so the read-only policy and role can only be a template.
- Other regions are blocked, so everything goes to `us-east-1`.
- Max `medium` instances and max 9 of them - this is the main limit for M2-M5 (see `quotas.md`).
- I can't create roles for my nodes, so they will use the existing service roles.
