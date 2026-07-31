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

// Tier2Pin is the name of the pin indicating that this
// snapshot should not be deleted until it is uploaded to tier2.
const Tier2Pin = "klio.io/tier2"
