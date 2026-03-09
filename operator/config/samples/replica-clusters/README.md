# Replica Clusters Example

This example demonstrates how to set up cross-datacenter PostgreSQL replication
using Klio with CloudNativePG (CNPG). The setup includes two datacenters,
`dc-a` and `dc-b`, represented by two different namespaces, with PostgreSQL
clusters that can replicate data between each other using Klio servers.

This setup is for demonstration and testing purposes only and is not intended
for production use.

## Components

### 1. Klio Servers
- `klioserver-dc-a`: Klio server in datacenter A
- `klioserver-dc-b`: Klio server in datacenter B

### 2. PostgreSQL Clusters
- `cluster-dc-a`: Primary PostgreSQL cluster
- `cluster-dc-b`: Replica PostgreSQL cluster

## Files Description

- `klio_v1alpha1_server_dc_a.yaml`: Klio server configuration for datacenter A
- `klio_v1alpha1_server_dc_b.yaml`: Klio server configuration for datacenter B
- `cluster-dc-a.yaml`: Primary PostgreSQL cluster configuration
- `cluster-dc-b.yaml`: Replica PostgreSQL cluster configuration

## Deployment Steps

### Step 1: Deploy Klio Servers

Deploy the Klio servers in both datacenters:

```bash
# Deploy Klio server in DC-A
kubectl apply -f klio_v1alpha1_server_dc_a.yaml

# Deploy Klio server in DC-B
kubectl apply -f klio_v1alpha1_server_dc_b.yaml
```

### Step 2: Deploy Primary Cluster (DC-A)

```bash
kubectl apply -f cluster-dc-a.yaml
```

Wait for the primary cluster to be ready before proceeding.

### Step 3: take a base backup (DC-A)

```bash
kubectl apply -f backup-dc-a.yaml
```

### Step 4: Deploy Replica Cluster (DC-B)

```bash
kubectl apply -f cluster-dc-b.yaml
```
