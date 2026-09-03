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

package wal

// Feedback represents the feedback we get from the Klio server
// when writing a WAL file.
type Feedback struct {
	// corresponding to flush_lsn in pg_stat_replication
	FlushLSN uint64

	// corresponding to write_lsn in pg_stat_replication
	WriteLSN uint64

	// corresponding to replay_lsn in pg_stat_replication
	ReplayLSN uint64
}
