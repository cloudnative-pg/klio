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

package klioclient

// BackupNameTagName is the name of the tag containing the
// backup name.
const BackupNameTagName = "klio.io/tag"

// BackupContentTagName is the name of the tag containing the
// snapshot content.
const BackupContentTagName = "klio.io/content"

// TablespaceNameTagName is the name of the tag containing the
// name of the tablespace.
const TablespaceNameTagName = "klio.io/tablespaceName"

// Tier2RelayAnnotationName is the metadata annotation recording whether the
// client asked for the backup to be relayed to tier2. Backups taken before
// the annotation existed carry none and are treated as relayed.
const Tier2RelayAnnotationName = "klio.io/tier2-relay"

// Tier2RelaySkipped is the Tier2RelayAnnotationName value of a backup that is
// not meant to reach tier2, so tier1 retention need not wait for it.
const Tier2RelaySkipped = "skipped"

// Tier2RelayRequested is the Tier2RelayAnnotationName value of a backup that
// must reach tier2 before tier1 retention can delete it.
const Tier2RelayRequested = "requested"
