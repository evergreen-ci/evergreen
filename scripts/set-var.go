package main

import (
	"flag"
	"time"

	"github.com/mongodb/grip"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
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

	session, err := mgo.DialWithTimeout("mongodb://localhost:27017", 2*time.Second)
	grip.CatchEmergencyFatal(err)
	c := session.DB(dbName).C(collection)

	grip.CatchEmergencyFatal(c.UpdateId(id, bson.M{"$set": bson.M{key: value}}))
	grip.Infof("set the value of '%s' to '%s' for document '%s' in collection '%s'", key, value, id, collection)
}
