package db

import (
	"strings"

	"github.com/pkg/errors"
	mgo "gopkg.in/mgo.v2"
)

func IsDuplicateKey(err error) bool {
	if err == nil {
		return false
	}

	err = errors.Cause(err)
	if mgo.IsDup(err) {
		return true
	}

	if strings.Contains(err.Error(), "duplicate key") {
		return true
	}

	return false
}
