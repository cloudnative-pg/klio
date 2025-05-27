package kopia

// kopiaIgnoreFileName is the name of the .kopiaignore
// file
const kopiaIgnoreFileName = ".kopiaignore"

// kopiaIgnoreContent is the content of the `.kopiaignore` file that is written
// to PGDATA before backing it up
const kopiaIgnoreContent = `
/pg_log/*
/log/*

/pg_xlog/*
/pg_wal/*

/global/pg_control

pgsql_tmp*
postgresql.auto.conf.tmp
current_logfiles.tmp
pg_internal.init
postmaster.pid
postmaster.opts
recovery.conf
standby.signal
pg_dynshmem/*
pg_notify/*
pg_replslot/*
pg_serial/*
pg_stat_tmp/*
pg_snapshots/*
pg_subtrans/*
`
