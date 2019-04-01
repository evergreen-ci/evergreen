package migrations

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"
)

//nolint: deadcode, megacheck, unused
const (
	migrationTaskCreateTime = "task-create-time"
	VersionCollection       = "versions"
)

// This migration updates the CreateTime field of patch tasks to be the push time of the base commit.
// nolint: unused
func taskCreateTimeGenerator(env anser.Environment, args migrationGeneratorFactoryOptions) (anser.Generator, error) { // nolint: deadcode, megacheck
	const migrationName = "task_create_time"

	if err := env.RegisterManualMigrationOperation(migrationName, makeTaskCreateTimeMigrationFunction(args.db)); err != nil {
		return nil, err
	}

	opts := model.GeneratorOptions{
		NS: model.Namespace{
			DB:         args.db,
			Collection: tasksCollection,
		},
		Limit: args.limit,
		Query: bson.M{
			"r": bson.M{"$in": evergreen.PatchRequesters},
		},
		JobID: args.id,
	}

	return anser.NewManualMigrationGenerator(env, opts, migrationName), nil
}

func makeTaskCreateTimeMigrationFunction(database string) db.MigrationOperation {
	const tasksCollection = "tasks"
	const VersionCollection = "versions"

	return func(session db.Session, rawD mgobson.RawD) error {
		defer session.Close()

		var id string
		var revision string
		var project string
		for _, raw := range rawD {
			switch raw.Name {
			case "_id":
				if err := raw.Value.Unmarshal(&id); err != nil {
					return errors.Wrap(err, "error unmarshaling task _id")
				}
			case "gitspec":
				if err := raw.Value.Unmarshal(&revision); err != nil {
					return errors.Wrap(err, "error unmarshaling task gitspec")
				}
			case "branch":
				if err := raw.Value.Unmarshal(&project); err != nil {
					return errors.Wrap(err, "error unmarshaling task branch")
				}
			}
		}

		// Find the base version for the patch.
		baseVersion := struct {
			CreateTime time.Time `bson:"create_time"`
		}{}

		query := bson.M{
			"identifier": project,
			"gitspec":    revision,
			"r": bson.M{
				"$in": evergreen.SystemVersionRequesterTypes,
			},
		}
		q := session.DB(database).C(VersionCollection).Find(query)
		if err := q.One(&baseVersion); err != nil {
			return errors.Wrap(err, "error finding base version")
		}

		// Update the task create time.
		updateQuery := bson.M{"$set": bson.M{"create_time": baseVersion.CreateTime}}
		if err := session.DB(database).C(tasksCollection).UpdateId(id, updateQuery); err != nil {
			return errors.Wrap(err, "error updating task create time")
		}

		// Update the old tasks create time.
		selectQuery := bson.M{"old_task_id": id}
		if _, err := session.DB(database).C(oldTasksCollection).UpdateAll(selectQuery, updateQuery); err != nil {
			return errors.Wrap(err, "error updating old tasks create time")
		}

		return nil
	}
}
