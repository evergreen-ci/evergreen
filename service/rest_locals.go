package service

import (
	"time"
)

const (
	// The resolution of times stored in the database
	TimePrecision = time.Duration(1 * time.Millisecond)
)

// Format of the JSON response when an error occurs
type responseError struct {
	Message string `json:"message"`
}
