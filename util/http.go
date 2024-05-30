package util

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// GetIntValue returns a form value as an integer
func GetIntValue(r *http.Request, valueKey string, defaultValue int) (int, error) {
	val := r.FormValue(valueKey)
	if val == "" {
		return defaultValue, nil
	}
	intVal, err := strconv.Atoi(val)
	if err != nil {
		return 0, errors.Wrapf(err, "'%s': cannot convert value '%s' to integer", valueKey, val)
	}
	return intVal, nil
}

// RespErrorf attempts to read a gimlet.ErrorResponse from the response body
// JSON. If successful, it returns the gimlet.ErrorResponse wrapped with the
// HTTP status code and the formatted error message. Otherwise, it returns an
// error message with the HTTP status and raw response body.
func RespErrorf(resp *http.Response, format string, args ...interface{}) error {
	if resp == nil {
		return errors.Errorf(format, args...)
	}
	wrapError := func(err error) error {
		err = errors.Wrapf(err, "HTTP status code %d", resp.StatusCode)
		return errors.Wrapf(err, format, args...)
	}

	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return wrapError(errors.Wrap(err, "reading response body"))
	}

	respErr := gimlet.ErrorResponse{}
	if err = json.Unmarshal(b, &respErr); err != nil {
		return wrapError(errors.Errorf("received response: %s", string(b)))
	}

	return wrapError(respErr)
}
