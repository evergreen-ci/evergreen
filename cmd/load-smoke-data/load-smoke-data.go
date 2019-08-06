package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
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
		path   string
		dbName string
	)

	flag.StringVar(&path, "path", filepath.Join(wd, "testdata", "smoke"), "load data from json files from these paths")
	flag.StringVar(&dbName, "dbName", "mci_smoke", "database name for directory")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017").SetConnectTimeout(5*time.Second))
	grip.EmergencyFatal(err)

	db := client.Database(dbName)
	grip.EmergencyFatal(db.Drop(ctx))

	var file *os.File
	files, err := getFiles(path)
	grip.EmergencyFatal(err)

	var doc map[string]interface{}
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

		scanner := bufio.NewScanner(file)
		count := 0
		for scanner.Scan() {
			doc = map[string]interface{}{}
			count++
			err = json.Unmarshal(scanner.Bytes(), &doc)
			if err != nil {
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
