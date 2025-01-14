# Useful debugging commands

## How to inspect the Kopia repository

```
/data # kopia repository connect filesystem --path=/data/data
Enter password to open repository:
Connected to repository.

/data # kopia snapshot ls --all
cluster-example@cluster-example:/wal/000000010000000000000004
  2025-01-14 16:25:17 UTC Ix3fbcf3d375314f52e9814cadc6eb13ee 16.8 MB -rw------- (latest-1,hourly-1,daily-1,weekly-1,monthly-1,annual-1)

cluster-example@cluster-example:/wal/000000010000000000000005
  2025-01-14 16:25:17 UTC Ix4ed069fcb5bb488f88b372b0a05631e0 16.8 MB -rw------- (latest-1,hourly-1,daily-1,weekly-1,monthly-1,annual-1)
/data #
```