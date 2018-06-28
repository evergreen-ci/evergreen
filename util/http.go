package util

import (
	"net/http"
	"strconv"
	"strings"

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
		return 0, errors.Errorf("%v: cannot convert %v to integer: %v", valueKey, val, err.Error())
	}
	return intVal, nil
}

// GetBoolValue returns a form value as an integer
func GetBoolValue(r *http.Request, valueKey string, defaultValue bool) (bool, error) {
	val := r.FormValue(valueKey)
	if val == "" {
		return defaultValue, nil
	}
	boolVal, err := strconv.ParseBool(val)
	if err != nil {
		return defaultValue, errors.Errorf("%v: cannot convert %v to boolean: %v", valueKey, val, err.Error())
	}
	return boolVal, nil
}

// GetStringArrayValue returns a form value as a string array
func GetStringArrayValue(r *http.Request, valueKey string, defaultValue []string) []string {
	val := r.FormValue(valueKey)
	if val == "" {
		return defaultValue
	}
	return strings.Split(val, ",")
}
