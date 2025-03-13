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
		dbName     string
		collection string
		id         string
		key        string
		value      string
	)

	flag.StringVar(&dbName, "dbName", "mci_smoke", "database name for directory")
	flag.StringVar(&collection, "collection", "", "name of collection")
	flag.StringVar(&id, "id", "", "_id of document")
	flag.StringVar(&key, "key", "", "key to set")
	flag.StringVar(&value, "value", "", "value of key")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017").SetConnectTimeout(2*time.Second))
	grip.EmergencyFatal(err)
	res, err := client.Database(dbName).Collection(collection).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{key: value}})
	grip.EmergencyFatal(err)
	if res.MatchedCount == 0 {
		grip.Warningf("no documents updated: %+v", res)
		os.Exit(2)
	}
	grip.Infof("set the value of '%s' for document '%s' in collection '%s'", key, id, collection)
	grip.Emergency(client.Disconnect(ctx))
}
