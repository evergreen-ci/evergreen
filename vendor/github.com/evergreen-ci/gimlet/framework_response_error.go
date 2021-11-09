package gimlet

import (
	"net/http"
)

// MakeTextErrorResponder takes an error object and converts
// it into a responder object that wrap's gimlet's ErrorResponse
// type. Since ErrorResponse implements the error interface, if you
// pass an ErrorResposne object, this function will propagate the
// status code specified in the ErrorResponse if that code is valid,
// otherwise the status code of the request and the response object
// will be 400.
func MakeTextErrorResponder(err error) Responder {
	return newResponder(err, http.StatusBadRequest, TEXT)
}

// MakeJSONErrorResponder takes an error object and converts
// it into a responder object that wrap's gimlet's ErrorResponse
// type. Since ErrorResponse implements the error interface, if you
// pass an ErrorResposne object, this function will propagate the
// status code specified in the ErrorResponse if that code is valid,
// otherwise the status code of the request and the response object
// will be 400.
func MakeJSONErrorResponder(err error) Responder {
	return newResponder(err, http.StatusBadRequest, JSON)
}

// MakeYAMLErrorResponder takes an error object and converts
// it into a responder object that wrap's gimlet's ErrorResponse
// type. Since ErrorResponse implements the error interface, if you
// pass an ErrorResposne object, this function will propagate the
// status code specified in the ErrorResponse if that code is valid,
// otherwise the status code of the request and the response object
// will be 400.
func MakeYAMLErrorResponder(err error) Responder {
	return newResponder(err, http.StatusBadRequest, YAML)
}

// MakeTextInternalErrorResponder takes an error object and converts
// it into a responder object that wrap's gimlet's ErrorResponse
// type. Since ErrorResponse implements the error interface, if you
// pass an ErrorResposne object, this function will propagate the
// status code specified in the ErrorResponse if that code is valid,
// otherwise the status code of the request and the response object
// will be 500.
func MakeTextInternalErrorResponder(err error) Responder {
	return newResponder(err, http.StatusInternalServerError, TEXT)
}

// MakeJSONInternalErrorResponder takes an error object and converts
// it into a responder object that wrap's gimlet's ErrorResponse
// type. Since ErrorResponse implements the error interface, if you
// pass an ErrorResposne object, this function will propagate the
// status code specified in the ErrorResponse if that code is valid,
// otherwise the status code of the request and the response object
// will be 500.
func MakeJSONInternalErrorResponder(err error) Responder {
	return newResponder(err, http.StatusInternalServerError, JSON)
}

// MakeYAMLInternalErrorResponder takes an error object and converts
// it into a responder object that wrap's gimlet's ErrorResponse
// type. Since ErrorResponse implements the error interface, if you
// pass an ErrorResposne object, this function will propagate the
// status code specified in the ErrorResponse if that code is valid,
// otherwise the status code of the request and the response object
// will be 500.
func MakeYAMLInternalErrorResponder(err error) Responder {
	return newResponder(err, http.StatusInternalServerError, YAML)
}
