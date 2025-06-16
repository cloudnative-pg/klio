# Demo notes

## Cross-compilation for Linux of the Klio binary

```
GOARCH=amd64 GOOS=linux go build -o klio main.go
kubectl cp $(pwd)/klio cluster-example-1:/controller/klio
```

## To enable compression on a Kopia repository

```
/data/pgdata $ kopia policy set --global --compression=zstd-fastest
```

## Show the latest WALs that have been received

```
kubectl exec -n klio -t deployment/klio-server -- sh -c 'watch -n 1 "ls /data/wals/cluster-example/\$(ls /data/wals/cluster-example/|tail -n 1)|sort|tail -n 20"'
```


## How to start a backup

```
/controller/klio backup --config=/controller/klio.yaml --debug
```

or

```
nohup /controller/klio backup --config=/controller/klio.yaml --debug > /controller/backup.log 2>&1
```