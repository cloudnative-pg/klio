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

package infrastructure

import "fmt"

// NoSingleResultSetError is returned when the number of result sets is not 1.
type NoSingleResultSetError struct {
	resultSets int
}

func (e *NoSingleResultSetError) Error() string {
	return fmt.Sprintf(
		"expected 1 result set from SHOW wal_segment_size, got %d",
		e.resultSets,
	)
}

// NoSingleRowError is returned when the number of result rows is not 1.
type NoSingleRowError struct {
	rows int
}

func (e *NoSingleRowError) Error() string {
	return fmt.Sprintf(
		"expected 1 result row from SHOW wal_segment_size, got %d",
		e.rows,
	)
}

// NoSingleColumnError is returned when the number of columns is not 1.
type NoSingleColumnError struct {
	columns int
}

func (e *NoSingleColumnError) Error() string {
	return fmt.Sprintf(
		"expected 1 result row from SHOW wal_segment_size, got %d",
		e.columns,
	)
}
