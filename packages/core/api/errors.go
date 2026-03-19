package api

import (
	"errors"
	"net/http"

	"boxer/sandbox"
)

// boxerError carries an HTTP status code alongside the error message.
type boxerError struct {
	status  int
	message string
}

func (e *boxerError) Error() string { return e.message }

// httpStatus maps a boxer error to the appropriate HTTP status code.
func httpStatus(err error) int {
	var be *boxerError
	if errors.As(err, &be) {
		return be.status
	}
	if errors.Is(err, sandbox.ErrTimeout) {
		return http.StatusRequestTimeout
	}
	if errors.Is(err, sandbox.ErrOutputLimit) {
		return http.StatusInsufficientStorage
	}
	return http.StatusInternalServerError
}
