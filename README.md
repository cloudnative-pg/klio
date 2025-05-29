# Klio - EDB Postgres Backup & Recovery Manager for CloudNativePG

The EDB Postgres Backup and Recovery Manager for CloudNativePG (codename “Klio”,
inspired by Clio, the muse of history, symbolizing the preservation and recovery
of past events, which resonates with database backup) is designed to set a new
benchmark in enterprise-grade backup and recovery for PostgreSQL databases on
Kubernetes, specifically. 

It effectively manages:

* The WAL archive for a specific PostgreSQL cluster
* The catalog of physical base backups for the same PostgreSQL cluster

These critical backup components are stored across two tiers:

* Tier 1 - Local volume: A local Persistent Volume Claim (PVC) within the same
  namespace as the `Cluster` resource, providing immediate, high-speed access.

* Tier 2 - Secondary Storage: An object store where data is asynchronously
  replicated from Tier 1. When directly or indirectly relayed outside the
  Kubernetes cluster, this tier ensures geographical redundancy and improved
  disaster recovery (DR) outcomes.

More information can be found in the [design
document](https://docs.google.com/document/d/1ZTJf7siLxLvH31X6eztY0xeySJ-QT9o6BCA9EAs1gxc/edit?tab=t.0)

## Development environment prerequisites

* A [dagger](https://dagger.io/) installation

## How to spawn up a testing environment

The following command will spawn up a new temporary Kubernetes environment with
CloudNativePG, the minimal cluster and **klio**:

```sh
dagger call kubernetes --source . terminal
```

## How to recreate the GRPC stub and skeleton

```sh
dagger call protoc --source . -o internal/grpc
```

## How to setup manually a Klio server

Given the following configuration file in `/home/klio/.klio.yaml`:

```yaml
klio_server:
  listen_address: 0.0.0.0:52000
  server_cert_path: /home/ubuntu/klio_data/server.crt
  server_key_path: /home/ubuntu/klio_data/server.key
  wal_path: /home/ubuntu/klio_data/wals
  password: thispassword
```

A new Klio WAL repository can be bootstrapped with:

```sh
openssl req -x509 -newkey rsa:4096 -sha256 -days 3650 \
  -nodes -keyout server.key -out server.crt -subj "/CN=klio-server" \
  -addext "subjectAltName=DNS:klio-server,IP:127.0.0.1"

~/klio initialize
```

To initialize the PGData repository:

```sh
~/kopia repository create filesystem --path=/home/ubuntu/klio_data/pgdata

~/kopia repository connect filesystem --path=/home/ubuntu/klio_data/pgdata

~/kopia policy set --global --compression=zstd
```

The WAL server can be started with:

```sh
~/klio serve
```

The Kopia server can be started with:

```sh
~/kopia server start \
  --address=https://0.0.0.0:51515 \
  --server-username=klio@cluster-example \
  --server-password=CHANGE_ME_KOPIA_PASSWORD \
  --server-control-username=klio \
  --server-control-password=CHANGE_ME_KOPIA_PASSWORD \
  --tls-cert-file=/home/ubuntu/klio_data/server.crt \
  --tls-key-file=/home/ubuntu/klio_data/server.key 
```

## How to setup manually a Klio client

Given the following configuration file in `/var/lib/postgresql/.klio.yaml`:

```yaml
client:
  klio:
    address: 52.29.253.97:52000
    cluster_name: cluster-example
    server_cert_path: /tmp/server.crt
    username: klio
    password: at2leeyomooduZu8KeR6
  kopia:
    base_url: https://52.29.253.97:51515
    hostname: cluster-example
    username: klio
    password: at2leeyomooduZu8KeR6
    trusted_server_certificate_fingerprint: DB89F46B52E26489AE85063B434144182588722AE6FE63AE557D489E319C6A9F

source:
  dsn: host=/var/run/postgresql user=postgres replication=yes application_name=klio
  slot: klio
```

Important: the certificate fingerprint can be found with:

```sh
openssl x509 -in /home/ubuntu/klio_data/server.crt -text -fingerprint -sha256
```
