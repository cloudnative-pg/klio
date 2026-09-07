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

// Manifest represents information about a single point-in-time filesystem snapshot.
type Manifest struct {
	// ID is the unique identifier of the snapshot.
	ID string `json:"id"`

	// Source contains information about what was snapshotted.
	Source SourceInfo `json:"source"`

	// RootEntry is the root directory entry of the snapshot.
	RootEntry *DirEntry `json:"rootEntry"`

	// Description is a user-provided description of the snapshot.
	Description string `json:"description"`

	// StartTime is when the snapshot started.
	StartTime string `json:"startTime"`

	// EndTime is when the snapshot completed.
	EndTime UTCTimestamp `json:"endTime"`

	// IncompleteReason describes why the snapshot is incomplete, if applicable.
	IncompleteReason string `json:"incomplete,omitempty"`

	// RetentionReasons lists the reasons why this snapshot is being retained.
	RetentionReasons []string `json:"-"`

	// Tags contains user-defined key-value pairs associated with the snapshot.
	Tags map[string]string `json:"tags,omitempty"`
}

// DirEntry represents a directory entry as stored in JSON stream.
type DirEntry struct {
	// Name is the name of the file or directory.
	Name string `json:"name,omitempty"`

	// FileSize is the size of the file in bytes.
	FileSize int64 `json:"size,omitempty"`

	// ModTime is the last modification time of the entry.
	ModTime UTCTimestamp `json:"mtime,omitempty"`

	// UserID is the numeric user ID of the file owner.
	UserID uint32 `json:"uid,omitempty"`

	// GroupID is the numeric group ID of the file owner.
	GroupID uint32 `json:"gid,omitempty"`

	// DirSummary contains summary information for directories.
	DirSummary *DirectorySummary `json:"summ,omitempty"`

	// ObjID is the ID of the root object.
	ObjID string `json:"obj,omitempty"`
}

// DirectorySummary represents summary information about a directory.
type DirectorySummary struct {
	// TotalFileSize is the total size of all files in the directory tree in bytes.
	TotalFileSize int64 `json:"size"`

	// TotalFileCount is the total number of files in the directory tree.
	TotalFileCount int64 `json:"files"`

	// TotalSymlinkCount is the total number of symbolic links in the directory tree.
	TotalSymlinkCount int64 `json:"symlinks"`

	// TotalDirCount is the total number of subdirectories in the directory tree.
	TotalDirCount int64 `json:"dirs"`

	// MaxModTime is the latest modification time found in the directory tree.
	MaxModTime UTCTimestamp `json:"maxTime"`

	// IncompleteReason describes why the directory summary is incomplete, if applicable.
	IncompleteReason string `json:"incomplete,omitempty"`

	// FatalErrorCount is the number of failed files.
	FatalErrorCount int `json:"numFailed"`

	// IgnoredErrorCount is the number of errors that were ignored.
	IgnoredErrorCount int `json:"numIgnoredErrors,omitempty"`
}

// SourceInfo represents the coordinates of what is being snapshotted.
type SourceInfo struct {
	// Host is the hostname where the snapshot was taken.
	Host string `json:"host"`

	// UserName is the username that took the snapshot.
	UserName string `json:"userName"`

	// Path is the filesystem path that was snapshotted.
	Path string `json:"path"`
}

func (ssi SourceInfo) String() string {
	if ssi.Host == "" && ssi.Path == "" && ssi.UserName == "" {
		return "(global)"
	}

	if ssi.Path == "" {
		return fmt.Sprintf("%v@%v", ssi.UserName, ssi.Host)
	}

	return fmt.Sprintf("%v@%v:%v", ssi.UserName, ssi.Host, ssi.Path)
}
