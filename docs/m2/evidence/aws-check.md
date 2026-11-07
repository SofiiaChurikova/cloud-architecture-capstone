# AWS check - manual deploy (2026-11-06)

Binaries from the GitHub Actions build on `milestone2`, uploaded to S3, stacks deployed with the same commands as the pipeline.

```
$ aws s3 mb s3://capstone-code-<account>
$ aws s3api get-bucket-versioning --bucket capstone-code-<account>
{ "Status": "Enabled" }

$ aws cloudformation deploy --stack-name capstone-network --template-file infra/network.yaml
Successfully created/updated stack - capstone-network
$ aws cloudformation deploy --stack-name capstone-app --template-file infra/app.yaml --parameter-overrides CoordinatorBinaryUrl=... ShardBinaryUrl=...
Successfully created/updated stack - capstone-app
$ aws cloudformation describe-stacks --stack-name capstone-app --query 'Stacks[0].Outputs' --output text
CoordinatorUrl  http://13.221.39.147:8080

$ aws ec2 describe-instances ...
|  shard-2    |  running  |  10.20.32.173  |
|  coordinator|  running  |  10.20.0.100   |
|  shard-1    |  running  |  10.20.21.114  |
```

The shards are in the private subnets (`10.20.16.0/20`, `10.20.32.0/20`) with no internet route. They downloaded the binary from S3 through the gateway endpoint and registered in the coordinator.

```
$ curl $URL/health
{"shards":2,"status":"ok"}

$ curl -X PUT $URL/tables/users -d '{"partition_key":"id"}'
{"name":"users","partition_key":"id"}

$ curl -i -X PUT $URL/tables/users/items/1 -d '{"id":"1","name":"a"}'
HTTP/1.1 200 OK
X-Shard-Id: shard-2

$ curl -i -X PUT $URL/tables/users/items/3 -d '{"id":"3","name":"b"}'
HTTP/1.1 200 OK
X-Shard-Id: shard-1

$ curl -i $URL/tables/users/items/1
HTTP/1.1 200 OK
X-Shard-Id: shard-2
{"id":"1","name":"a"}

$ curl -i $URL/tables/users/items/3
HTTP/1.1 200 OK
X-Shard-Id: shard-1
{"id":"3","name":"b"}

$ curl -I $URL/tables/users/items/1
HTTP/1.1 200 OK
X-Shard-Id: shard-2

$ curl -i -X DELETE $URL/tables/users/items/1
HTTP/1.1 204 No Content
X-Shard-Id: shard-2

$ curl -i $URL/tables/users/items/1
HTTP/1.1 404 Not Found
X-Shard-Id: shard-2
item not found

$ curl $URL/internal/shards
[{"id":"shard-1","addr":"http://10.20.21.114:8081"},{"id":"shard-2","addr":"http://10.20.32.173:8081"}]
```

Same result as the local run: key 1 goes to shard-2, key 3 to shard-1, reads go to the same shard as the write.
