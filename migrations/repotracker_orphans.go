package migrations

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/pkg/errors"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	buildCollection   = "builds"
	versionCollection = "versions"
	taskCollection    = "tasks"

	requesterKey = "r"
	statusKey    = "status"
)

func orphanedVersionCleanupGenerator(env anser.Environment, db string, limit int) (anser.Generator, error) {
	const migrationName = "clean_orphaned_versions"

	if err := env.RegisterManualMigrationOperation(migrationName, makeOrphanedVersionCleanup(db)); err != nil {
		return nil, err
	}

	opts := model.GeneratorOptions{
		NS: model.Namespace{
			DB:         db,
			Collection: versionCollection,
		},
		Query: bson.M{
			requesterKey: evergreen.RepotrackerVersionRequester,
			statusKey:    evergreen.VersionCreated, // TODO is this true?
		},
		Limit: limit,
		JobID: "migration-versions-orphans",
	}

	return anser.NewManualMigrationGenerator(env, opts, migrationName), nil
}

func makeOrphanedVersionCleanup(database string) db.MigrationOperation {
	const (
		idKey            = "_id"
		buildStatusesKey = "build_variants_status"
		buildIDsKey      = "builds"
	)
	return func(session db.Session, rawD bson.RawD) error {
		defer session.Close()

		bstatuses := []version.BuildStatus{}
		builds := []string{}
		versionID := ""

		for i := range rawD {
			switch rawD[i].Name {
			case idKey:
				if err := rawD[i].Value.Unmarshal(&versionID); err != nil {
					return errors.Wrap(err, "error unmarshaling id")
				}

			case buildStatusesKey:
				if err := rawD[i].Value.Unmarshal(&bstatuses); err != nil {
					return errors.Wrap(err, "error unmarshaling build statuses")
				}

			case buildIDsKey:
				if err := rawD[i].Value.Unmarshal(&builds); err != nil {
					return errors.Wrap(err, "error unmarshaling builds")
				}
			}
		}

		buildMap := map[string]bool{}

		newBuilds := []string{}
		for _, buildID := range builds {
			_, ok := buildMap[buildID]
			if !ok {
				query := session.DB(database).C(buildCollection).FindId(buildID)
				err := query.One(nil)
				if err == mgo.ErrNotFound {
					continue
				}
				if err != nil {
					return errors.Wrapf(err, "error fetching build %s", buildID)
				}
				buildMap[buildID] = true
			}

			newBuilds = append(newBuilds, buildID)
		}

		newBuildStatuses := []version.BuildStatus{}
		for _, bstatus := range bstatuses {
			_, ok := buildMap[bstatus.BuildId]
			if !ok {
				query := session.DB(database).C(buildCollection).FindId(bstatus.BuildId)
				err := query.One(nil)
				if err == mgo.ErrNotFound {
					continue
				}
				if err != nil {
					return errors.Wrapf(err, "error fetching build %s", bstatus.BuildId)
				}
				buildMap[bstatus.BuildId] = true
			}

			newBuildStatuses = append(newBuildStatuses, bstatus)
		}

		if len(newBuilds) == 0 && len(newBuildStatuses) == 0 {
			return session.DB(database).C(versionCollection).RemoveId(versionID)
		}

		update := bson.M{
			"$set": bson.M{
				buildIDsKey:      newBuilds,
				buildStatusesKey: newBuildStatuses,
			},
		}

		return session.DB(database).C(versionCollection).UpdateId(versionID, update)
	}
}

func orphanedBuildCleanupGenerator(env anser.Environment, db string, limit int) (anser.Generator, error) {
	const migrationName = "clean_orphaned_builds"

	if err := env.RegisterManualMigrationOperation(migrationName, makeOrphanedBuildCleanup(db)); err != nil {
		return nil, err
	}

	opts := model.GeneratorOptions{
		NS: model.Namespace{
			DB:         db,
			Collection: buildCollection,
		},
		Query: bson.M{
			requesterKey: evergreen.RepotrackerVersionRequester,
			statusKey:    evergreen.BuildCreated, // TODO is this true?
		},
		Limit: limit,
		JobID: "migration-builds-orphans",
	}

	return anser.NewManualMigrationGenerator(env, opts, migrationName), nil
}

// orpganedBuildCleanup, given a build, will:
// 1. If it's version doesn't exist, remove the build and it's tasks
// 2. Check it's task cache, and ensure that all listed tasks exist, removing
//    any that do not exist
func makeOrphanedBuildCleanup(database string) db.MigrationOperation {
	const (
		idKey      = "_id"
		tasksKey   = "tasks"
		versionKey = "version"
		buildIDKey = "build_id"
	)
	return func(session db.Session, rawD bson.RawD) error {
		defer session.Close()

		cachedTasks := []build.TaskCache{}
		versionID := ""
		buildID := ""

		for i := range rawD {
			switch rawD[i].Name {
			case idKey:
				if err := rawD[i].Value.Unmarshal(&buildID); err != nil {
					return errors.Wrap(err, "error unmarshaling id")
				}

			case tasksKey:
				if err := rawD[i].Value.Unmarshal(&cachedTasks); err != nil {
					return errors.Wrap(err, "error unmarshaling tasks")
				}

			case versionKey:
				if err := rawD[i].Value.Unmarshal(&versionID); err != nil {
					return errors.Wrap(err, "error unmarshaling version")
				}
			}
		}

		query := session.DB(database).C(versionCollection).FindId(versionID)
		err := query.One(nil)

		if err == mgo.ErrNotFound {
			err := session.DB(database).C(taskCollection).Remove(bson.M{
				buildIDKey: buildID,
			})
			if err != nil && err != mgo.ErrNotFound {
				return errors.Wrap(err, "error deleting tasks for build")
			}

			return errors.WithStack(session.DB(database).C(buildCollection).RemoveId(buildID))
		}
		if err != nil {
			return errors.WithStack(err)
		}

		cachedTaskIDs := []string{}
		cachedTasksMap := map[string]*build.TaskCache{}
		for i := range cachedTasks {
			cachedTaskIDs = append(cachedTaskIDs, cachedTasks[i].Id)
			cachedTasksMap[cachedTasks[i].Id] = &cachedTasks[i]
		}

		tasks := []task.Task{}
		query = session.DB(database).C(taskCollection).Find(bson.M{
			idKey: bson.M{
				"$in": cachedTaskIDs,
			},
		})
		if err := query.All(&tasks); err != nil {
			return err
		}
		if len(tasks) == 0 {
			return errors.WithStack(session.DB(database).C(buildCollection).RemoveId(buildID))
		}
		if len(tasks) == len(cachedTasks) {
			return nil
		}

		newTasksCache := []build.TaskCache{}
		for i := range tasks {
			if tc, ok := cachedTasksMap[tasks[i].Id]; ok {
				newTasksCache = append(newTasksCache, *tc)
			}
		}

		update := bson.M{
			"$set": bson.M{
				tasksKey: newTasksCache,
			},
		}

		return errors.WithStack(session.DB(database).C(buildCollection).UpdateId(buildID, update))
	}
}
