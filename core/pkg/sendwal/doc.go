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

// Package sendwal implements a Postgres physical-replication WAL receiver:
// it negotiates a starting position, manages the replication slot lifecycle,
// streams WAL data via START_REPLICATION, and hands received bytes to a
// caller-supplied buffer.Handler sink.
//
// This package does not know about Klio, or about any specific destination
// for the received WAL data: callers plug in a ReplicationCoordinator to
// negotiate streaming with whatever system ultimately stores the WAL, and a
// buffer.Handler (see the buffer subpackage) to actually receive the bytes.
//
// .. note::
//
//	The receiver is opinionated in one respect: it will create the
//	replication slot it is configured to use if it does not already exist.
//	This matches what any backup tool needs, but is not configurable yet.
package sendwal
