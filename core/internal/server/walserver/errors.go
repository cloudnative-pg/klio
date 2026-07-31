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

package walserver

import "errors"

var (
	errReadOnly         = errors.New("read only repository")
	errEmptyClusterName = errors.New("empty cluster name")
	errEmptyWALName     = errors.New("empty WAL name")
	errEmptySegmentSize = errors.New("empty segment size")

	// errNotWALSegment is returned when a file is not a real WAL segment
	// (e.g. .history, .backup or .partial) and therefore must not drive the
	// latest_written_* metrics.
	errNotWALSegment = errors.New("not a WAL segment")

	// ErrParsingClientCACertificate is raised when we couldn't parse
	// the client CA certificate file.
	ErrParsingClientCACertificate = errors.New("parsing client CA certificate file failed")
)
