package api

import (
	"errors"
	"net/http"
)

// boxerError carries an HTTP status code alongside the error message.
type boxerError struct {
	status  int
	message string
}

func (e *boxerError) Error() string { return e.message }

// Sentinel error types for HTTP status mapping.
var (
	errTimeout     = errors.New("execution timed out")
	errOutputLimit = errors.New("output limit exceeded")
)

// httpStatus maps a boxer error to the appropriate HTTP status code.
func httpStatus(err error) int {
	var be *boxerError
	if errors.As(err, &be) {
		return be.status
	}
	if errors.Is(err, errTimeout) {
		return http.StatusRequestTimeout
	}
	if errors.Is(err, errOutputLimit) {
		return http.StatusInsufficientStorage
	}
	return http.StatusInternalServerError
}
