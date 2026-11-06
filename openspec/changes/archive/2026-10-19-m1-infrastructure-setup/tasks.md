# Tasks

## 1. Decisions

- [x] 1.1 Write ADR-0001: choice of IaC tool
- [x] 1.2 Write ADR-0002: stack layout
- [x] 1.3 Write ADR-0003: CIDR addressing scheme

## 2. Sandbox permissions (LabRole -> voclabs)

- [x] 2.1 Read the role and its policies in the console / CLI (no LabRole in this sandbox; we act as `voclabs`)
- [x] 2.2 Check actions with the policy simulator and real calls
- [x] 2.3 Write the results to `docs/m1/labrole.md`

## 3. Read-only policy and role

- [x] 3.1 Write `infra/iam-readonly.yaml`
- [x] 3.2 Validate it with `aws cloudformation validate-template`
- [x] 3.3 Try to deploy it in the sandbox and save the error

## 4. Network

- [x] 4.1 Write `infra/network.yaml`
- [x] 4.2 Deploy the network stack and check it reaches CREATE_COMPLETE
- [x] 4.3 Check route tables: public -> IGW, private -> local only

## 5. Quotas and budget

- [x] 5.1 Write quotas that can limit M2 to `docs/m1/quotas.md`
- [x] 5.2 Calculate the M2 budget for 2 shards and compare it with the credit ceiling in `docs/m1/budget.md`

## 6. Recreatability

- [x] 6.1 Delete the network stack and deploy it again from the template
- [x] 6.2 Record timings and problems in `docs/m1/recreate.md`

## 7. Defense

- [x] 7.1 Run an AI design review and record accepted / refuted points
- [x] 7.2 Update `README.md`