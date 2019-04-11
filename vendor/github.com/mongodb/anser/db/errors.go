package db

import (
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
	mgo "gopkg.in/mgo.v2"
)

var errNotFound = errors.New("document not found")

func ResultsNotFound(err error) bool {
	return errors.Cause(err) == mgo.ErrNotFound || errors.Cause(err) == errNotFound || errors.Cause(err) == mongo.ErrNoDocuments
}
