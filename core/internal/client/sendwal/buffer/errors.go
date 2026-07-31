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

package buffer

import "fmt"

// UnexpectedWalDataOffsetError is the error returned when
// the WAL data offset is not the expected one.
type UnexpectedWalDataOffsetError struct {
	offset   uint64
	expected uint64
}

func (e *UnexpectedWalDataOffsetError) Error() string {
	return fmt.Sprintf("Unexpected WAL data offset: %08x, expected: %08x", e.offset, e.expected)
}

// UnopenedFileForWALError is the error returned when a WAL
// record is received without a WAL file open.
type UnopenedFileForWALError struct {
	offset uint64
}

func (e *UnopenedFileForWALError) Error() string {
	return fmt.Sprintf("received write-ahead log record for offset %v with no file open", e.offset)
}
