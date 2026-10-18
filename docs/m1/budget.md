# M1 - Task 7: Budget for M2 vs. sandbox credits

**Credit limit: $20** - the lab panel shows "Used $0 of $20".
Prices are for `us-east-1`, on-demand, Linux. 1 month = 730 hours.

## M2: 1 coordinator + 2 shards on EC2

| What | Details | $/hour | $/month (24/7) |
|---|---|---|---|
| Coordinator | 1 x `t3.small` | 0.0208 | 15.18 |
| Shards | 2 x `t3.micro` (0.0104 each) | 0.0208 | 15.18 |
| Disks | 3 x 8 GB `gp2` ($0.10 per GB per month, the sandbox allows only gp2) | 0.0033 | 2.40 |
| Public IPv4 | 1 address for the coordinator ($0.005/h) | 0.0050 | 3.65 |
| Network from M1 | VPC, subnets, IGW, route tables | 0 | 0 |
| **Total** | | **~$0.05** | **~$36.40** |

I don't use a NAT gateway: it costs about $33 per month, so it alone would eat the whole budget in less than 3 weeks.

## Compared with $20

| Scenario | Cost | Fits? |
|---|---|---|
| M2 running 24/7 for a month | ~$36.40 | **No**, $20 is enough for about 16 days |
| M2 running only during lab sessions | ~$0.05/h, so ~$0.15 for a 3-hour session | **Yes**, about 400 hours in total |
| M3 preview (1 coordinator + 6 replicas on `t3.micro`, 7 x 8 GB gp2) | ~$0.096/h (~$70/month) | only about 210 hours |

## Conclusions

- M2 can't run 24/7 on $20. The budget makes sense only in hours of sessions, not in months.
- That's how the sandbox works anyway: everything is deleted after a 3-hour session, and I rebuild the stacks from code next time.
- The network stack is free, only compute costs money. That's also why the network and the app are in separate stacks (ADR-0002).
- M3 almost doubles the price per hour, so I need to check the budget again before M3.
