package gimlet

import (
	"fmt"
	"net/http"

	"github.com/mongodb/grip/message"
)

// ErrorResponse implements the Error() interface and provides a
// common type for routes to use for returning payloads in error
// conditions.
//
// ErrorResponse also implements grip's message.Composer interface
// which can simplify some error reporting on the client side.
type ErrorResponse struct {
	StatusCode   int    `bson:"status" json:"status" yaml:"status"`
	Message      string `bson:"message" json:"message" yaml:"message"`
	message.Base `bson:"metadata" json:"metadata" yaml:"metadata"`
}

func (e ErrorResponse) Error() string {
	return fmt.Sprintf("%d (%s): %s", e.StatusCode, http.StatusText(e.StatusCode), e.Message)
}

func (e ErrorResponse) String() string   { return e.Error() }
func (e ErrorResponse) Raw() interface{} { return e }
func (e ErrorResponse) Loggable() bool   { return e.StatusCode > 399 }
