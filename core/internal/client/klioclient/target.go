package klioclient

import "fmt"

// Target is used to point a Kopia transaction to the set of snapshots
// having the specified Hostname and Username.
type Target struct {
	// Hostname is the hostname of the snapshot, as in the
	// <username>@<hostname> snapshot indicator.
	Hostname string

	// Username is the name of the user that took the snapshot, as in the
	// <username>@<hostname> snapshot indicator.
	Username string
}

// String formats this target as the string that the Kopia CLI
// would expect.
func (t Target) String() string {
	return fmt.Sprintf("%s@%s", t.Username, t.Hostname)
}
