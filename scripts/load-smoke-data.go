package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"time"

	mgo "gopkg.in/mgo.v2"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
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
	grip.CatchEmergencyFatal(err)
	var (
		path   string
		dbName string
	)

	flag.StringVar(&path, "path", filepath.Join(wd, "testdata", "smoke"), "load data from json files from these paths")
	flag.StringVar(&dbName, "dbName", "mci_smoke", "database name for directory")
	flag.Parse()

	session, err := mgo.DialWithTimeout("mongodb://localhost:27017", 2*time.Second)
	grip.CatchEmergencyFatal(err)
	db := session.DB(dbName)

	var file *os.File
	files, err := getFiles(path)
	grip.CatchEmergencyFatal(err)

	doc := map[string]interface{}{}
	catcher := grip.NewBasicCatcher()
	for _, fn := range files {
		file, err = os.Open(fn)
		if err != nil {
			catcher.Add(errors.Wrap(err, "problem opening file"))
			continue
		}
		defer file.Close()

		collName := strings.Split(filepath.Base(fn), ".")[0]
		collection := db.C(collName)
		catcher.Add(collection.DropCollection())

		scanner := bufio.NewScanner(file)
		count := 0
		for scanner.Scan() {
			count++
			err = json.Unmarshal(scanner.Bytes(), &doc)
			if err != nil {
				catcher.Add(errors.Wrapf(err, "problem reading document #%d from %s", count, fn))
				continue
			}

			catcher.Add(collection.Insert(doc))
		}
		catcher.Add(scanner.Err())
		grip.Infof("imported %d documents into %s", count, collName)
	}

	grip.CatchEmergencyFatal(catcher.Resolve())
}
