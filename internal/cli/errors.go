package cli

import "fmt"

// UsageError marks a CLI usage mistake that should exit with code 2.
type UsageError struct {
	Err error
}

func (e UsageError) Error() string {
	if e.Err == nil {
		return "usage error"
	}
	return e.Err.Error()
}

func (e UsageError) Unwrap() error {
	return e.Err
}

func (e UsageError) ExitCode() int {
	return 2
}

func usageErrorf(format string, args ...any) error {
	return UsageError{Err: fmt.Errorf(format, args...)}
}
