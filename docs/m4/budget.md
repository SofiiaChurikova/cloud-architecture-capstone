# M4 - budget

M4 adds one instance for Prometheus and Grafana (ADR-0015). Everything else is the same as M3.

Prices: `us-east-1`, on-demand, Linux.

| What | Details | $/hour |
|---|---|---|
| Coordinator | 1 x `t3.micro` | 0.0104 |
| Replicas | 6 x `t3.micro` | 0.0624 |
| Monitoring | 1 x `t3.micro` (Prometheus + Grafana) | 0.0104 |
| Disks | 8 x 8 GB `gp2` ($0.10 per GB per month) | 0.0088 |
| Public IPv4 | coordinator + monitoring | 0.0100 |
| Cross-AZ traffic | same as M3, plus scrapes: 8 targets every 10 s, a few KB each | ~0 |
| **Total** | | **~$0.102** |

Lecture 2 says observability costs money too. Here it's ~$0.016/h (the instance, its disk and its IP), about 16% of the stack. Prometheus keeps 3 days of data on the 8 GB disk, for 8 targets that's far below the disk size.

## Compared with $20

- One 3-hour session is about $0.31.
- $20 is about 195 hours of M4.
- 24/7 for a month it would be ~$74, so M4 also runs only during sessions.
