package service

import (
	"net/http"
	"strconv"
)

func getIntValue(r *http.Request, valueKey string, defaultValue int) (int, error) {
	val := r.FormValue(valueKey)
	if val == "" {
		return defaultValue, nil
	}
	return strconv.Atoi(val)
}
