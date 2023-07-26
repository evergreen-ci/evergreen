package db

import (
	"strings"

	"github.com/evergreen-ci/evergreen/db/mgo"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
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

func IsErrorCode(err error, errorCode int) bool {
	var mongoError mongo.CommandError
	if errors.As(err, &mongoError) && mongoError.HasErrorCode(errorCode) {
		return true
	}
	return false
}

// ErrorCode constants for mongo errors

const FACET_PIPELINE_STAGE_TOO_LARGE_CODE = 4031700 // https://github.com/mongodb/mongo/blob/a1732172ed5d66d98582ea1059c0ede9d8cd5065/src/mongo/db/pipeline/document_source_facet.cpp#L165
