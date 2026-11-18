# M3 - budget

M3 has 7 instances instead of 3. Lecture 2 also says replication traffic between AZs is the cost people forget first, so it's counted too.

Prices: `us-east-1`, on-demand, Linux.

| What | Details | $/hour |
|---|---|---|
| Coordinator | 1 x `t3.micro` | 0.0104 |
| Replicas | 6 x `t3.micro` | 0.0624 |
| Disks | 7 x 8 GB `gp2` ($0.10 per GB per month), the log lives here | 0.0077 |
| Public IPv4 | 1 for the coordinator | 0.0050 |
| Cross-AZ traffic | $0.01 per GB each way, see below | ~0 |
| **Total** | | **~$0.086** |

## Cross-AZ traffic

Every shard has replicas in both AZs (ADR-0013), so every write is copied to at least one other AZ, and half of the requests from the coordinator go to the other AZ too. A YCSB run (10 000 inserts + 25 000 updates, ~1 KB each) is about 35 000 x 2 KB = ~70 MB across AZs, ~$0.0014. For my sessions it's close to nothing, but it grows with every write.

## Compared with $20

- One 3-hour session is about $0.26, 2x the M2 price.
- $20 is about 230 hours of M3, still enough.
- 24/7 for a month it would be ~$63, so M3 also runs only during sessions.
