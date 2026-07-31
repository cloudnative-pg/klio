/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package kopia

// kopiaIgnoreFileName is the name of the .kopiaignore
// file.
const kopiaIgnoreFileName = ".kopiaignore"

// kopiaIgnoreContent is the content of the `.kopiaignore` file that is written
// to PGDATA before backing it up.
const kopiaIgnoreContent = `
/pg_log/*
/log/*

/pg_xlog/*
/pg_wal

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
