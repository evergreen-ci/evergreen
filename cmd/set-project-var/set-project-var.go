package main

import (
	"flag"
	"time"

	"github.com/mongodb/grip"
	mgo "gopkg.in/mgo.v2"
	"go.mongodb.org/mongo-driver/bson"
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

	session, err := mgo.DialWithTimeout("mongodb://localhost:27017", 2*time.Second)
	grip.EmergencyFatal(err)
	collection := session.DB(dbName).C("project_vars")

	grip.EmergencyFatal(collection.UpdateId(project, bson.M{"$set": bson.M{"vars." + key: value}}))
	grip.Infof("set the value of '%s' for project '%s'", key, project)
}
