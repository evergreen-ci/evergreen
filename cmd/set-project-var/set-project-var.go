package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/evergreen-ci/evergreen/cloud/parameterstore"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	var (
		dbName  string
		project string
		key     string
		value   string
	)

	flag.StringVar(&dbName, "dbName", "mci_smoke", "database name for directory")
	flag.StringVar(&project, "project", "evergreen", "name of project")
	flag.StringVar(&key, "key", "", "key to set")
	flag.StringVar(&value, "value", "", "value of key")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017").SetConnectTimeout(5*time.Second))
	grip.EmergencyFatal(errors.Wrap(err, "connecting to DB"))

	paramName := fmt.Sprintf("/evg-test/vars/%s/%s", project, key)

	db := client.Database(dbName)

	res, err := db.Collection(model.ProjectVarsCollection).UpdateOne(ctx,
		bson.M{"_id": project},
		bson.M{"$addToSet": bson.M{"parameters": bson.M{
			"name":           key,
			"parameter_name": paramName,
		}},
			"$setOnInsert": bson.M{
				"_id": project,
			},
		}, options.Update().SetUpsert(true))
	grip.EmergencyFatal(errors.Wrap(err, "updating project var parameter"))
	if res.ModifiedCount+res.UpsertedCount == 0 {
		grip.Warningf("no project var documents updated: %+v", res)
		os.Exit(2)
	}
	now := time.Now()

	fp := fakeparameter.FakeParameter{
		Name:        paramName,
		Value:       value,
		LastUpdated: now,
	}
	res, err = db.Collection(fakeparameter.Collection).ReplaceOne(ctx, bson.M{
		"_id": paramName,
	}, fp, options.Replace().SetUpsert(true))
	grip.EmergencyFatal(errors.Wrap(err, "updating fake parameter"))
	if res.ModifiedCount+res.UpsertedCount == 0 {
		grip.Warningf("no fake parameter documents updated: %+v", res)
		os.Exit(3)
	}

	rec := parameterstore.ParameterRecord{
		Name:        paramName,
		LastUpdated: now,
	}
	res, err = client.Database(dbName).Collection(parameterstore.Collection).ReplaceOne(ctx, bson.M{
		"_id": paramName,
	}, rec, options.Replace().SetUpsert(true))
	grip.EmergencyFatal(errors.Wrap(err, "updating parameter record"))
	if res.ModifiedCount+res.UpsertedCount == 0 {
		grip.Warningf("no parameter record documents updated: %+v", res)
		os.Exit(4)
	}

	grip.EmergencyFatal(err)
	if res.ModifiedCount+res.UpsertedCount == 0 {
		grip.Warningf("no documents updated: %+v", res)
		os.Exit(5)
	}
	grip.Infof("set the value of '%s' for project '%s'", key, project)
}
