# Tasks

## 1. Decisions

- [x] 1.1 ADR-0004: API protocol and format
- [x] 1.2 ADR-0005: hash function and number of virtual nodes
- [x] 1.3 ADR-0006: partition key strategy
- [x] 1.4 ADR-0007: how shards get the code (S3 endpoint)
- [x] 1.5 ADR-0008: CI/CD with GitHub Actions
- [x] 1.6 ADR-0009: where the coordinator keeps its metadata

## 2. Code

- [x] 2.1 `api/openapi.yaml`
- [x] 2.2 Coordinator alone: tables + CREATE / READ / EXISTS / DELETE
- [x] 2.3 Shard: in-memory store + register in the coordinator
- [x] 2.4 Consistent hashing ring + test that keys are spread evenly
- [x] 2.5 Coordinator sends writes and reads to shards through the ring
- [x] 2.6 Run coordinator + 2 shards locally and check with curl

## 3. AWS

- [ ] 3.1 S3 endpoint in `infra/network.yaml`
- [ ] 3.2 `infra/app.yaml`: coordinator + 2 shards on EC2
- [ ] 3.3 GitHub Actions pipeline
- [ ] 3.4 YCSB benchmark, save results to `docs/m2/benchmark.md`

## 4. Defense

- [ ] 4.1 AI design review
- [ ] 4.2 README
