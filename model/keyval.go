package model

import (
	"github.com/evergreen-ci/evergreen/db"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const KeyValCollection = "keyval_plugin"

type KeyVal struct {
	Key   string `bson:"_id" json:"key"`
	Value int64  `bson:"value" json:"value"`
}

func (kv *KeyVal) Inc() error {
	key := kv.Key
	change := adb.Change{
		Update: bson.M{
			"$inc": bson.M{"value": 1},
		},
		ReturnNew: true,
		Upsert:    true,
	}

	_, err := db.FindAndModify(KeyValCollection, bson.M{"_id": key}, nil, change, kv)

	if err != nil {
		return errors.Wrapf(err, "incrementing key '%s'", key)
	}

	return nil
}
