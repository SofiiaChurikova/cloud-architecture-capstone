# M2 - budget again

Lecture 8 asks to count the M2 budget again: 2 shards + coordinator + a metadata store (RDS or DynamoDB). In M1 (`docs/m1/budget.md`) I counted the coordinator as `t3.small`, in the end it's a `t3.micro`. And there is no RDS or DynamoDB, metadata stays in memory (ADR-0009).

Prices: `us-east-1`, on-demand, Linux.

| What | Details | $/hour |
|---|---|---|
| Coordinator | 1 x `t3.micro` | 0.0104 |
| Shards | 2 x `t3.micro` | 0.0208 |
| Disks | 3 x 8 GB `gp2` ($0.10 per GB per month) | 0.0033 |
| Public IPv4 | 1 for the coordinator | 0.0050 |
| S3 bucket | 2 binaries ~10 MB + YCSB results | ~0 |
| S3 gateway endpoint | free | 0 |
| Metadata store | none, in memory (ADR-0009). RDS `db.t4g.micro` would be +0.016/h | 0 |
| **Total** | | **~$0.039** |

## Compared with $20

- One 3-hour session is about $0.12.
- $20 is about 500 hours of M2 running, so it's fine.
- 24/7 for a month it would be ~$29, still more than $20, so I keep running it only in sessions.
- With RDS for the metadata it would be ~$0.055/h, 40% more for 2 tiny lists - one more reason for ADR-0009.
