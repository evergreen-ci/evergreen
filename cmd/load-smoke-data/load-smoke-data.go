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
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
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
		return nil, errors.Wrapf(err, "finding import files in path '%s'", root)
	}

	return out, nil
}

func insertFileDocsToDB(ctx context.Context, fn string, db *mongo.Database) error {
	var file *os.File
	file, err := os.Open(fn)
	if err != nil {
		return errors.Wrap(err, "opening file")
	}
	defer file.Close()

	collName := strings.TrimSuffix(filepath.Base(fn), ".json")
	collection := db.Collection(collName)
	switch collName {
	case task.Collection:
		if _, err = collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
			{
				Keys: task.ActivatedTasksByDistroIndex,
			},
			{
				Keys: task.DurationIndex,
			},
		}); err != nil {
			return errors.Wrap(err, "creating task indexes")
		}
	case host.Collection:
		if _, err = collection.Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: host.StatusIndex},
			{Keys: host.StartedByStatusIndex},
		}); err != nil {
			return errors.Wrap(err, "creating host index")
		}
	}
	scanner := bufio.NewScanner(file)
	// Set the max buffer size to the max size of a Mongo document (16MB).
	scanner.Buffer(make([]byte, 4096), 16*1024*1024)
	count := 0
	for scanner.Scan() {
		count++
		bytes := scanner.Bytes()

		var doc bson.D
		if err := bson.UnmarshalExtJSON(bytes, false, &doc); err != nil {
			return errors.Wrapf(err, "reading document #%d from file '%s'", count, fn)
		}

		if _, err := collection.InsertOne(ctx, doc); err != nil {
			return errors.Wrapf(err, "inserting doc #%d from file '%s'", count, fn)
		}
	}

	if err := scanner.Err(); err != nil {
		return errors.Wrapf(err, "scanning documents from file '%s'", fn)
	}

	grip.Infof("imported %d documents into %s", count, collName)

	return nil
}

func writeDummyGridFSFile(ctx context.Context, db *mongo.Database) error {
	bucket, err := gridfs.NewBucket(db, &options.BucketOptions{Name: utility.ToStringPtr(patch.GridFSPrefix)})
	if err != nil {
		return errors.Wrap(err, "creating GridFS bucket")
	}
	_, err = bucket.UploadFromStream(gridFSFileID, strings.NewReader("sample_patch"), nil)
	if err != nil {
		return errors.Wrap(err, "writing GridFS file")
	}

	grip.Infof("wrote %s.%s to gridFS", patch.GridFSPrefix, gridFSFileID)

	return nil
}

func getFilesFromPathAndInsert(ctx context.Context, path string, db *mongo.Database) error {
	files, err := getFiles(path)
	if err != nil {
		return errors.Wrap(err, "getting files")
	}

	for _, fn := range files {
		fileInfo, err := os.Stat(fn)
		if err != nil {
			return errors.Wrapf(err, "getting file info for %s", fn)
		}
		switch mode := fileInfo.Mode(); {
		case mode.IsRegular():
			if err := insertFileDocsToDB(ctx, fn, db); err != nil {
				return errors.Wrapf(err, "adding DB documents from file '%s'", fn)
			}
		}
	}

	return nil
}

// buildAmboyIndexes builds the required indexes necessary to run the
// application's Amboy queues.
func buildAmboyIndexes(ctx context.Context, dbURI string, db *mongo.Database) error {
	queueOpts := getAmboyQueueOptions(dbURI, db)
	// The queue is only set up to initiate the remote queue index builds.
	rq, err := queue.NewMongoDBQueue(ctx, queueOpts)
	if err != nil {
		return errors.Wrap(err, "creating remote queue")
	}
	rq.Close(ctx)

	queueGroupOpts := getAmboyQueueOptions(dbURI, db)
	queueGroupOpts.DB.UseGroups = true
	groupOpts := queue.MongoDBQueueGroupOptions{
		DefaultQueue: queueGroupOpts,
	}
	queueGroup, err := queue.NewMongoDBSingleQueueGroup(ctx, groupOpts)
	if err != nil {
		return errors.Wrap(err, "creating remote queue group")
	}
	// The queue is only generated within the queue group to initiate the queue
	// group index builds.
	if _, err := queueGroup.Get(ctx, "fake-group"); err != nil {
		return errors.Wrap(err, "creating queue within remote queue group")
	}
	if err := queueGroup.Close(ctx); err != nil {
		return errors.Wrap(err, "closing queue group")
	}

	grip.Info("successfully built required Amboy indexes")

	return nil
}

// getAmboyQueueOptions returns the options to set up the Amboy queue to create
// indexes.
func getAmboyQueueOptions(dbURI string, db *mongo.Database) queue.MongoDBQueueOptions {
	dbOpts := queue.DefaultMongoDBOptions()
	dbOpts.URI = dbURI
	dbOpts.DB = db.Name()
	dbOpts.Client = db.Client()
	dbOpts.Collection = evergreen.DefaultAmboyQueueName
	dbOpts.Format = amboy.BSON2
	dbOpts.SkipQueueIndexBuilds = false
	dbOpts.SkipReportingIndexBuilds = false
	return queue.MongoDBQueueOptions{
		DB:         &dbOpts,
		NumWorkers: utility.ToIntPtr(1),
	}
}

func main() {
	wd, err := os.Getwd()
	grip.EmergencyFatal(err)
	var (
		path        string
		dbName      string
		amboyDBName string
	)

	flag.StringVar(&path, "path", filepath.Join(wd, "smoke", "internal", "testdata", "db"), "load data from json files from these paths")
	flag.StringVar(&dbName, "dbName", "mci_smoke", "database name for directory")
	flag.StringVar(&amboyDBName, "amboyDBName", "amboy_smoke", "name of the Amboy DB to use")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const dbURI = "mongodb://localhost:27017"

	clientOptions := options.Client().ApplyURI(dbURI).SetConnectTimeout(5 * time.Second)
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

	amboyDB := client.Database(amboyDBName)
	grip.EmergencyFatal(amboyDB.Drop(ctx))

	catcher := grip.NewBasicCatcher()
	catcher.Wrap(buildAmboyIndexes(ctx, dbURI, amboyDB), "building Amboy indexes")
	catcher.Wrapf(getFilesFromPathAndInsert(ctx, path, db), "adding DB documents from file path '%s'", path)
	catcher.Wrap(writeDummyGridFSFile(ctx, db), "writing dummy file to GridFS")

	catcher.Add(client.Disconnect(ctx))
	grip.EmergencyFatal(catcher.Resolve())
}
