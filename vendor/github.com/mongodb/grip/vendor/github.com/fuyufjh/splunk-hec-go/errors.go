package hec

import (
	"errors"
)

// Response is response message from HEC. For example, `{"text":"Success","code":0}`.
type Response struct {
	Text  string          `json:"text"`
	Code  int             `json:"code"`
	AckID *int            `json:"ackId"` // Use a pointer so we can differentiate between a 0 and an ack ID not being specified
	Acks  map[string]bool `json:"acks"`  // Splunk returns ack IDs as strings rather than ints
}

// Response status codes
const (
	StatusSuccess              = 0
	StatusTokenDisabled        = 1
	StatusTokenRequired        = 2
	StatusInvalidAuthorization = 3
	StatusInvalidToken         = 4
	StatusNoData               = 5
	StatusInvalidDataFormat    = 6
	StatusIncorrectIndex       = 7
	StatusInternalServerError  = 8
	StatusServerBusy           = 9
	StatusChannelMissing       = 10
	StatusInvalidChannel       = 11
	StatusEventFieldRequired   = 12
	StatusEventFieldBlank      = 13
	StatusAckDisabled          = 14
)

func retriable(code int) bool {
	return code == StatusServerBusy || code == StatusInternalServerError
}

var ErrEventTooLong = errors.New("Event length is too long")
