package repository

// IncorrectWALNameError is raised when a WAL file name is not correct.
type IncorrectWALNameError struct {
	WalName string
}

// NewIncorrectWALNameError creates a new NewIncorrectWALNameError structure.
func NewIncorrectWALNameError(name string) *IncorrectWALNameError {
	return &IncorrectWALNameError{
		WalName: name,
	}
}

// Error implements the error interface.
func (e *IncorrectWALNameError) Error() string {
	return "incorrect WAL file name: " + e.WalName
}
