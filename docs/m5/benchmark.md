# M5 benchmark - YCSB with autoscaling

Same workload as M2 / M3 (`bench/workload-a.properties`: 50% read / 50% update, zipfian, 10 000 records), from CloudShell in us-east-1. The autoscaler runs on my laptop (`cmd/autoscaler`) with the default rules: up when write p99 > 60 ms for 2 min, down when < 20 ms or no traffic for 5 min, 5 min cooldown (ADR-0020).

## How I run it

In CloudShell, as in M3 (Go in `/tmp`, `bench/` uploaded again, `./bench/build-ycsb.sh`):

```bash
URL=http://<coordinator-ip>:8080; TOKEN=...
COMMON="-P bench/workload-a.properties -p kvstore.url=$URL -p kvstore.token=$TOKEN -p table=m5"
./go-ycsb load kvstore $COMMON -p threadcount=16

# 1. Slow growth: 100 -> 1600 ops/s, 3 minutes each step
for t in 100 200 400 800 1600; do
  ./go-ycsb run kvstore $COMMON -p threadcount=32 -p target=$t -p maxexecutiontime=180 -p operationcount=10000000 | tee slow-$t.txt
done

# 2. Spike: 5 min quiet (100 ops/s), then 5 min at full speed
./go-ycsb run kvstore $COMMON -p threadcount=4  -p target=100 -p maxexecutiontime=300 -p operationcount=10000000 | tee spike-low.txt
./go-ycsb run kvstore $COMMON -p threadcount=64 -p maxexecutiontime=300 -p operationcount=10000000 | tee spike-high.txt

# 3. Long high load: 15 min at full speed
./go-ycsb run kvstore $COMMON -p threadcount=64 -p maxexecutiontime=900 -p operationcount=10000000 | tee long.txt
```

On my laptop, at the same time:

```bash
KV_TOKEN=$TOKEN ./bin/autoscaler --prometheus http://<monitoring-ip>:9090 --coordinator $URL | tee autoscaler.log
```

Latency, throughput and active nodes over time are on the Grafana dashboard (panels "Write latency", "Throughput", "Scaling: shards on the ring and active nodes", "Scaling: shard move").

## Results

TODO after the AWS session.

| Pattern | Shards over time | Write p99 before / after scale up (ms) | Throughput before / after (ops/s) | Notes |
|---|---|---|---|---|
| Slow growth | | | | |
| Spike | | | | |
| Long high load | | | | |

## Compared with M2

TODO: M2 baseline (2 shards, no replicas): update 2985 ops/s, avg 2.64 ms, p99 21.2 ms; read avg 2.63 ms, p99 21.0 ms.
