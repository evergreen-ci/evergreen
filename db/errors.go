package db

import (
	"strings"

	anserDB "github.com/mongodb/anser/db"
	"go.mongodb.org/mongo-driver/mongo"
	mgo "gopkg.in/mgo.v2"
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

func ResultsNotFound(err error) bool {
	return anserDB.ResultsNotFound(err) || err == mongo.ErrNoDocuments
}
