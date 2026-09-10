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

package sendwal

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgproto3"
)

// UnexpectedMessageError is raised from the WAL receiver got a CopyData message
// of unknown type.
type UnexpectedMessageError struct {
	msg pgproto3.BackendMessage
}

// NewUnexpectedMessageError creates a new unexpected copy data message.
func NewUnexpectedMessageError(msg pgproto3.BackendMessage) *UnexpectedMessageError {
	return &UnexpectedMessageError{
		msg: msg,
	}
}

// Error implements the error interface.
func (e *UnexpectedMessageError) Error() string {
	return fmt.Sprintf("unexpected message, type=%+v", e.msg)
}

// UnexpectedCopydataMessageError is raised from the WAL receiver got a CopyData message
// of unknown type.
type UnexpectedCopydataMessageError struct {
	messageLength int
	messageType   byte
}

// NewUnexpectedCopydataMessageError creates a new unexpected copy data message.
func NewUnexpectedCopydataMessageError(msg []byte) *UnexpectedCopydataMessageError {
	if len(msg) == 0 {
		return &UnexpectedCopydataMessageError{
			messageLength: 0,
			messageType:   0,
		}
	}

	return &UnexpectedCopydataMessageError{
		messageLength: len(msg),
		messageType:   msg[0],
	}
}

// Error implements the error interface.
func (e *UnexpectedCopydataMessageError) Error() string {
	return fmt.Sprintf("unexpected copy data message, type=%v length=%v", e.messageType, e.messageLength)
}
