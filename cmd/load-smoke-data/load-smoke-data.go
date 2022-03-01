package main

import (
	"bufio"
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/gridfs"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const gridFSFileID = "5e4ff3ab850e6136624eaf95"

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

func insertFileDocsToDb(ctx context.Context, fn string, catcher grip.Catcher, db *mongo.Database, logsDb *mongo.Database) {
	var file *os.File
	file, err := os.Open(fn)
	if err != nil {
		catcher.Add(errors.Wrap(err, "problem opening file"))
		return
	}
	defer file.Close()

	collName := strings.Split(filepath.Base(fn), ".")[0]
	collection := db.Collection(collName)
	// task_logg collection belongs to the logs db
	if collName == model.TaskLogCollection {
		collection = logsDb.Collection(collName)
	}
	if collName == testresult.Collection { // add the necessary test results index
		_, err = collection.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: testresult.TestResultsIndex})
		catcher.Add(err)
	}
	if collName == task.Collection { // add the necessary tasks index
		_, err = collection.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: task.ActivatedTasksByDistroIndex})
		catcher.Add(err)
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

func writeDummyGridFSFile(ctx context.Context, catcher grip.Catcher, db *mongo.Database) {
	bucket, err := gridfs.NewBucket(db, &options.BucketOptions{Name: utility.ToStringPtr(patch.GridFSPrefix)})
	if err != nil {
		catcher.Add(errors.Wrap(err, "problem creating GridFS bucket"))
		return
	}
	_, err = bucket.UploadFromStream(gridFSFileID, strings.NewReader("sample_patch"), nil)
	if err != nil {
		catcher.Add(errors.Wrap(err, "problem writing GridFS file"))
		return
	}

	grip.Infof("wrote %s.%s to gridFS", patch.GridFSPrefix, gridFSFileID)
}

func getFilesFromPathAndInsert(ctx context.Context, path string, catcher grip.Catcher, db *mongo.Database, logsDb *mongo.Database) {
	files, err := getFiles(path)
	grip.EmergencyFatal(err)

	for _, fn := range files {
		fileInfo, err := os.Stat(fn)
		if err != nil {
			catcher.Add(errors.Wrapf(err, "problem getting file info for %s", fn))
		}
		switch mode := fileInfo.Mode(); {
		case mode.IsDir():
			getFilesFromPathAndInsert(ctx, fn, catcher, db, logsDb)
		case mode.IsRegular():
			insertFileDocsToDb(ctx, fn, catcher, db, logsDb)
		}
	}
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

	clientOptions := options.Client().ApplyURI("mongodb://localhost:27017").SetConnectTimeout(5 * time.Second)
	envAuth := os.Getenv(evergreen.MongodbAuthFile)
	if envAuth != "" {
		ymlUser, ymlPwd, err := evergreen.GetAuthFromYAML(envAuth)
		if err != nil {
			grip.Error(errors.Wrapf(err, "problem getting auth info from %s, trying to connect to db without auth", envAuth))
		}
		if err == nil && ymlUser != "" {
			credential := options.Credential{
				Username: ymlUser,
				Password: ymlPwd,
			}
			clientOptions.SetAuth(credential)
		}
	}
	client, err := mongo.Connect(ctx, clientOptions)
	grip.EmergencyFatal(err)

	db := client.Database(dbName)
	grip.EmergencyFatal(db.Drop(ctx))

	logsDb := client.Database(logsDbName)
	catcher := grip.NewBasicCatcher()

	getFilesFromPathAndInsert(ctx, path, catcher, db, logsDb)
	writeDummyGridFSFile(ctx, catcher, db)

	catcher.Add(client.Disconnect(ctx))
	grip.EmergencyFatal(catcher.Resolve())
}
