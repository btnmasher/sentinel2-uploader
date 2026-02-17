package client

import "errors"

type HTTPStatusError struct {
	StatusCode int
	Status     string
}

func (e *HTTPStatusError) Error() string {
	if e == nil {
		return "http request failed"
	}
	if e.Status != "" {
		return e.Status
	}
	return "http request failed"
}

func IsUnauthorized(err error) bool {
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) {
		return false
	}
	return statusErr.StatusCode == 401 || statusErr.StatusCode == 403
}
