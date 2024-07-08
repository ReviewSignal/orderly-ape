package client

import (
	"fmt"

	"github.com/go-resty/resty/v2"
)

type StatusError struct {
	Code     int
	Response *resty.Response
}

func NewStatusError(response *resty.Response) *StatusError {
	if response == nil {
		return nil
	}

	return &StatusError{
		Code:     response.StatusCode(),
		Response: response,
	}
}

func (s *StatusError) StatusCode() int {
	return s.Code
}

func (s *StatusError) Error() string {
	return fmt.Sprintf("unexpected status code: %d", s.Code)
}

func IsNotFound(err error) bool {
	if status, ok := err.(*StatusError); ok {
		return status.Code == 404
	}
	return false
}

func IgnoreNotFound(err error) error {
	if IsNotFound(err) {
		return nil
	}
	return err
}
