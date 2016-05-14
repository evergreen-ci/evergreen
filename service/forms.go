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

func getSkipAndLimit(r *http.Request, defaultSkip int, defaultLimit int) (int, int) {
	skip, err := getIntValue(r, "skip", defaultSkip)
	if err != nil {
		skip = defaultSkip
	}

	limit, err := getIntValue(r, "limit", defaultLimit)
	if err != nil {
		limit = defaultLimit
	}
	return skip, limit
}
