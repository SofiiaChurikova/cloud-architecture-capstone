# Local run - coordinator + 2 shards (task 2.6)

Coordinator on :8080, shard-1 on :8081, shard-2 on :8082, all started from `go build` binaries.

```
$ curl -s localhost:8080/health
{"shards":2,"status":"ok"}

$ curl -s -X PUT localhost:8080/tables/users -d '{"partition_key":"id"}'
{"name":"users","partition_key":"id"}

$ for k in 1 2 3 42; do curl -s -o /dev/null -D - -X PUT localhost:8080/tables/users/items/$k -d "{\"id\":\"$k\",\"name\":\"user$k\"}" | grep X-Shard; done
X-Shard-Id: shard-2      # key 1
X-Shard-Id: shard-2      # key 2
X-Shard-Id: shard-1      # key 3
X-Shard-Id: shard-1      # key 42

$ curl -s -i localhost:8080/tables/users/items/42
HTTP/1.1 200 OK
X-Shard-Id: shard-1
{"id":"42","name":"user42"}

$ curl -s -I localhost:8080/tables/users/items/42         # EXISTS
HTTP/1.1 200 OK

$ curl -s -X DELETE -w "%{http_code}\n" localhost:8080/tables/users/items/42
204
$ curl -s -o /dev/null -w "%{http_code}\n" localhost:8080/tables/users/items/42
404
$ curl -s -I localhost:8080/tables/users/items/42
HTTP/1.1 404 Not Found

$ curl -s localhost:8080/internal/shards
[{"id":"shard-1","addr":"http://localhost:8081"},{"id":"shard-2","addr":"http://localhost:8082"}]
$ curl -s localhost:8081/health; curl -s localhost:8082/health
{"id":"shard-1","items":1}
{"id":"shard-2","items":2}
```

The read of key 42 went to the same shard as the write (shard-1). After the delete shard-1 has 1 item left (key 3), shard-2 has 2 (keys 1, 2).

`go vet ./...` and `go test ./...` pass (ring test: each of 2 shards gets 45-55% of 100 000 keys).
