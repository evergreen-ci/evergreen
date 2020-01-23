package db

import (
	"strings"

	"gopkg.in/mgo.v2"
)

func IsDuplicateKey(err error) bool {
	if err == nil {
		return false
	}

	if mgo.IsDup(err) {
		return true
	}

	if strings.Contains(err.Error(), "duplicate key") {
		return true
	}

	return false
}
