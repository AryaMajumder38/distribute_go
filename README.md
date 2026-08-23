# proglog

A distributed, replicated commit log service built in Go — a Kafka-inspired log you can produce to, consume from, replicate across a cluster, and deploy to Kubernetes.

This project is a complete implementation of [*Distributed Services with Go*](https://pragprog.com/titles/tjgo/distributed-services-with-go/) by Travis Jeffery, built chapter by chapter from scratch.

## What It Does

At its core, proglog is an **append-only commit log**: records are appended, assigned sequential offsets, persisted to disk, and read back by offset. On top of that core, it adds everything needed to run it as a production-style distributed system:

- **Persistent storage engine** — segmented log files with memory-mapped sparse indexes (`internal/log`)
- **gRPC API** — unary and streaming produce/consume, plus cluster metadata endpoints (`api/v1`, `internal/server`)
- **Security** — TLS encryption, mutual TLS authentication via client certificates, and ACL-based authorization backed by Casbin (`internal/auth`, `internal/config`)
- **Observability** — structured request logs (Zap), metrics and distributed traces (OpenCensus)
- **Service discovery** — decentralized cluster membership and failure detection with Serf; servers automatically find each other (`internal/discovery`)
- **Consensus & replication** — leader election and log replication with Raft; every record is committed to a majority of nodes before being acknowledged (`internal/log/distributed.go`)
- **Client-side load balancing** — a custom gRPC resolver discovers servers at runtime and a custom picker routes produces to the leader while round-robin balancing consumes across followers (`internal/loadbalance`)
- **Deployment** — Docker image, Helm chart for Kubernetes (StatefulSet + headless Service), gRPC health probes, and service-per-pod cloud load balancers via Metacontroller

## Project Layout

```
├── api/v1                  protobuf definitions and generated code
│   ├── log.proto           Record, Log service (Produce/Consume/GetServers...)
│   └── error.go            typed gRPC error (ErrOffsetOutOfRange)
├── cmd/
│   ├── proglog             full agent CLI used as the Docker entrypoint
│   ├── server              minimal standalone gRPC server
│   └── getservers          helper that prints cluster members
├── internal/
│   ├── agent               wires log + server + discovery into one process
│   ├── auth                Casbin-backed authorizer
│   ├── config              cert/config file paths and TLS setup
│   ├── discovery           Serf membership wrapper
│   ├── loadbalance         custom gRPC resolver and picker
│   ├── log                 storage engine: store, index, segment, log,
│   │                       plus the Raft-replicated DistributedLog
│   └── server              gRPC service implementation, interceptors
├── deploy/proglog          Helm chart (StatefulSet, headless Service,
│                           health probes, service-per-pod hooks)
├── test                    CFSSL certificate configs, Casbin model/policy
├── Dockerfile              multi-stage build with grpc_health_probe
└── Makefile                compile, test, gencert, build-docker targets
```

## Requirements

- Go 1.23+
- [protoc](https://protobuf.dev/) with `protoc-gen-go` and `protoc-gen-go-grpc`
- [CFSSL](https://github.com/cloudflare/cfssl) (`brew install cfssl`) for test certificates
- Optional, for deployment: Docker, [kind](https://kind.sigs.k8s.io/), Helm

## Getting Started

### Run the tests

```bash
make init && make gencert   # generates certs and ACL configs into ~/.proglog
make test                   # go test -race ./...
```

The test suite covers the storage engine unit tests, gRPC client/server round trips, authorization rules, multi-node Serf membership, a three-node Raft cluster (election, replication, node removal), and an end-to-end agent cluster test.

### Run a single server

```bash
go run ./cmd/server         # gRPC server listening on :8080
```

Or run the full agent CLI:

```bash
make gencert
go run ./cmd/proglog \
  --node-name=0 \
  --bootstrap=true \
  --acl-model-file=$HOME/.proglog/model.conf \
  --acl-policy-file=$HOME/.proglog/policy.csv \
  --server-tls-cert-file=$HOME/.proglog/server.pem \
  --server-tls-key-file=$HOME/.proglog/server-key.pem \
  --server-tls-ca-file=$HOME/.proglog/ca.pem \
  --peer-tls-cert-file=$HOME/.proglog/root-client.pem \
  --peer-tls-key-file=$HOME/.proglog/root-client-key.pem \
  --peer-tls-ca-file=$HOME/.proglog/ca.pem
```

### Form a cluster

Start the first node with `--bootstrap=true`, then point additional nodes at it:

```bash
go run ./cmd/proglog --node-name=1 --bind-addr=127.0.0.1:8402 --rpc-port=8402 \
  --start-join-addrs=127.0.0.1:8401 ...
```

Serf handles membership; Raft elects a leader and replicates writes. Clients connect using the custom resolver scheme:

```go
conn, _ := grpc.NewClient("proglog:///127.0.0.1:8400", opts...)
client := api.NewLogClient(conn)
// produces route to the Raft leader, consumes round-robin across followers
```

## Deploying to Kubernetes

```bash
# build the image and load it into a local kind cluster
make build-docker
kind create cluster
kind load docker-image github.com/travisjeffery/proglog:0.0.1

# install the chart (3-node cluster by default)
helm install proglog deploy/proglog

# verify all members joined the cluster
kubectl port-forward pod/proglog-0 8400 8400 &
go run ./cmd/getservers -addr=:8400
```

For cloud deployments, push the image to your registry, install
[Metacontroller](https://metacontroller.github.io/), then enable per-pod load
balancers:

```bash
helm install proglog deploy/proglog \
  --set image.repository=gcr.io/$PROJECT_ID/proglog \
  --set service.lb=true
```

## API

The `log.v1.Log` gRPC service exposes:

| RPC | Type | Description |
|---|---|---|
| `Produce` | unary | Append a record; returns its assigned offset |
| `Consume` | unary | Read the record at an offset |
| `ProduceStream` | bidirectional stream | Stream records in with acks out |
| `ConsumeStream` | server stream | Follow the log from an offset, blocking on new records |
| `GetServers` | unary | List cluster members and which one is the leader |

Consuming past the end of the log returns a typed error with gRPC status `NotFound` and a localized message detail.

## Notes vs. the Book

The book was written against 2020-era libraries; this implementation keeps the architecture faithful but uses current APIs where needed:

- `grpc.NewClient` and `credentials/insecure` instead of deprecated `grpc.Dial`/`WithInsecure`
- `hashicorp/raft-boltdb/v2` (bbolt) — the original boltdb crashes under `-race` checkptr on arm64
- `casbin/v2`, `status.Code` instead of `grpc.Code`, `os.CreateTemp`/`MkdirTemp` instead of `ioutil`
- Modern gRPC resolver/balancer interfaces (`resolver.Target.URL`, embedded `balancer.SubConn` mocks)

## License

Private learning project. Book rights belong to Travis Jeffery and The Pragmatic Bookshelf.
