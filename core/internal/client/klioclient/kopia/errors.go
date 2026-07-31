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

import "fmt"

// NoBackupFoundError is raised when the requested backup has not been found
// in the Kopia manifests store.
type NoBackupFoundError struct {
	hostName   string
	backupName string
}

func newNoBackupFoundError(hostName string, backupName string) NoBackupFoundError {
	return NoBackupFoundError{
		hostName:   hostName,
		backupName: backupName,
	}
}

func (err NoBackupFoundError) Error() string {
	return fmt.Sprintf("backup %q for host %q not found", err.backupName, err.hostName)
}

// NoSnapshotFoundError is raised when the requested snapshot has not been found
// in the Kopia manifests store.
type NoSnapshotFoundError struct {
	hostname string
	tags     map[string]string
}

func newNoSnapshotFound(hostname string, tags map[string]string) NoSnapshotFoundError {
	return NoSnapshotFoundError{
		hostname: hostname,
		tags:     tags,
	}
}

func (err NoSnapshotFoundError) Error() string {
	return fmt.Sprintf("snapshot not found for tags %q and hostname %q", err.tags, err.hostname)
}
