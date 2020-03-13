package main

import (
	"bufio"
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func getFiles(root string) ([]string, error) {
	out := []string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if strings.HasSuffix(path, ".json") {
			out = append(out, path)
		}

		return nil
	})

	if err != nil {
		return nil, errors.Wrapf(err, "problem finding import files in %s", root)
	}

	return out, nil
}

func main() {
	wd, err := os.Getwd()
	grip.EmergencyFatal(err)
	var (
		path       string
		dbName     string
		logsDbName string
	)

	flag.StringVar(&path, "path", filepath.Join(wd, "testdata", "smoke"), "load data from json files from these paths")
	flag.StringVar(&dbName, "dbName", "mci_smoke", "database name for directory")
	flag.StringVar(&logsDbName, "logsDbName", "logs", "logs database name for directory")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017").SetConnectTimeout(5*time.Second))
	grip.EmergencyFatal(err)

	db := client.Database(dbName)
	grip.EmergencyFatal(db.Drop(ctx))

	logsDb := client.Database(logsDbName)

	var file *os.File
	files, err := getFiles(path)
	grip.EmergencyFatal(err)

	catcher := grip.NewBasicCatcher()
	for _, fn := range files {
		file, err = os.Open(fn)
		if err != nil {
			catcher.Add(errors.Wrap(err, "problem opening file"))
			continue
		}
		defer file.Close()

		collName := strings.Split(filepath.Base(fn), ".")[0]
		collection := db.Collection(collName)
		// task_logg collection belongs to the logs db
		if collName == model.TaskLogCollection {
			collection = logsDb.Collection(collName)
		}
		scanner := bufio.NewScanner(file)
		count := 0
		for scanner.Scan() {
			count++
			bytes := scanner.Bytes()
			// if the current collection is task_logg, delete from the collection the id that
			// is about to be inserted so we can avoid dropping the collection completely.
			if collName == model.TaskLogCollection {
				taskLog := model.TaskLog{}
				if err = bson.UnmarshalExtJSON(bytes, false, &taskLog); err != nil {
					catcher.Add(errors.Wrapf(err, "problem reading document #%d from %s into TaskLog struct", count, fn))
					continue
				}
				_, err := collection.DeleteOne(ctx, bson.M{"_id": taskLog.Id})
				catcher.Add(err)
			}
			doc := bson.D{}
			if err = bson.UnmarshalExtJSON(bytes, false, &doc); err != nil {
				catcher.Add(errors.Wrapf(err, "problem reading document #%d from %s", count, fn))
				continue
			}

			_, err := collection.InsertOne(ctx, doc)
			catcher.Add(err)
		}
		catcher.Add(scanner.Err())
		grip.Infof("imported %d documents into %s", count, collName)
	}

	catcher.Add(client.Disconnect(ctx))
	grip.EmergencyFatal(catcher.Resolve())
}
