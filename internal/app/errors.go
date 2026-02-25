package app

import "fmt"

const (
	ExitSuccess     = 0
	ExitUserError   = 1
	ExitPartial     = 2
	ExitAuthFailure = 3
	ExitIOFailure   = 4
)

type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("exit code %d", e.Code)
	}
	return e.Err.Error()
}

func WrapExit(code int, err error) error {
	if err == nil {
		return nil
	}
	return &ExitError{Code: code, Err: err}
}

func ExitCode(err error) int {
	if err == nil {
		return ExitSuccess
	}
	exitErr, ok := err.(*ExitError)
	if ok {
		return exitErr.Code
	}
	return ExitUserError
}
