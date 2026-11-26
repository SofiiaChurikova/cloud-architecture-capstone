# M5 - budget

M5 doesn't add new kinds of resources. The number of shard instances changes between 3 and 6 (ADR-0020).

Prices: `us-east-1`, on-demand, Linux.

| What | 1 shard | 2 shards |
|---|---|---|
| Coordinator, 1 x `t3.micro` | 0.0104 | 0.0104 |
| Monitoring, 1 x `t3.micro` | 0.0104 | 0.0104 |
| Replicas, 3 or 6 x `t3.micro` | 0.0312 | 0.0624 |
| Disks, 5 or 8 x 8 GB `gp2` | 0.0055 | 0.0088 |
| Public IPv4, coordinator + monitoring | 0.0100 | 0.0100 |
| **Total $/hour** | **~0.068** | **~0.102** |

Scaling down to 1 shard when there is no load saves about a third of the cost. A shard move copies ~half of the keys once (10 000 keys of ~1 KB = ~10 MB, about $0.0001 of cross-AZ traffic).

## Compared with $20

- A 3-hour session at 2 shards is ~$0.31; the same session mostly at 1 shard is ~$0.20-0.25.
- 24/7 for a month: ~$74 at 2 shards, ~$50 at 1 shard. Still only during sessions.
