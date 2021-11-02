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

	if mgo.IsDup(errors.Cause(err)) {
		return true
	}

	if strings.Contains(errors.Cause(err).Error(), "duplicate key") {
		return true
	}

	return false
}

func IsDocumentLimit(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(errors.Cause(err).Error(), "an inserted document is too large")
}
