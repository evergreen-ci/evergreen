package main

import (
	"context"
	"flag"
	"os"
	"time"

	"github.com/mongodb/grip"
	"go.mongodb.org/mongo-driver/bson"
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
	flag.StringVar(&project, "project", "62fe27ad9dbe3250c1a51028", "name of project")
	flag.StringVar(&key, "key", "", "key to set")
	flag.StringVar(&value, "value", "", "value of key")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017").SetConnectTimeout(5*time.Second))
	grip.EmergencyFatal(err)

	res, err := client.Database(dbName).Collection("project_vars").UpdateOne(ctx, bson.M{"_id": "62fe27ad9dbe3250c1a51028"}, bson.M{"$set": bson.M{"vars." + key: value}})
	grip.EmergencyFatal(err)
	if res.MatchedCount == 0 {
		grip.Warningf("no documents updated: %+v", res)
		os.Exit(2)
	}
	grip.Infof("set the value of '%s' for project '%s'", key, project)
	grip.Emergency(client.Disconnect(ctx))
}
