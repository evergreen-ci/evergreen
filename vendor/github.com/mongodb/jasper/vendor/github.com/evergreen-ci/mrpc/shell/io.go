package shell

import (
	"github.com/pkg/errors"
)

func intOK(ok bool) int {
	if ok {
		return 1
	}
	return 0
}

// ErrorResponse represents a response indicating whether the operation was okay
// and errors, if any.
type ErrorResponse struct {
	OK           int    `bson:"ok"`
	ErrorMessage string `bson:"errmsg,omitempty"`
}

// MakeErrorResponse returns an ErrorResponse with the given ok status and error
// message, if any.
func MakeErrorResponse(ok bool, err error) ErrorResponse {
	resp := ErrorResponse{OK: intOK(ok)}
	if err != nil {
		resp.ErrorMessage = err.Error()
	}
	return resp
}

// MakeSuccessResponse returns an ErrorResponse that is ok and has no error.
func MakeSuccessResponse() ErrorResponse {
	return ErrorResponse{OK: intOK(true)}
}

func (r ErrorResponse) SuccessOrError() error {
	if r.ErrorMessage != "" {
		return errors.New(r.ErrorMessage)
	}
	if r.OK == 0 {
		return errors.New("response was not ok")
	}
	return nil
}

// whatsMyURIResponse represents a response indicating the service's URI.
type whatsMyURIResponse struct {
	ErrorResponse ErrorResponse `bson:"error_response,inline"`
	You           string        `bson:"you"`
}

func makeWhatsMyURIResponse(uri string) whatsMyURIResponse {
	return whatsMyURIResponse{You: uri, ErrorResponse: MakeSuccessResponse()}
}

// buildInfoResponse represents a response indicating the service's build
// information.
type buildInfoResponse struct {
	ErrorResponse ErrorResponse `bson:"error_response,inline"`
	Version       string        `bson:"version"`
}

func makeBuildInfoResponse(version string) buildInfoResponse {
	return buildInfoResponse{Version: version, ErrorResponse: MakeSuccessResponse()}
}

// getLogResponse represents a response indicating the service's currently
// available logs.
type getLogResponse struct {
	ErrorResponse ErrorResponse `bson:"error_response,inline"`
	Log           []string      `bson:"log"`
}

func makeGetLogResponse(log []string) getLogResponse {
	return getLogResponse{Log: log, ErrorResponse: MakeSuccessResponse()}
}
