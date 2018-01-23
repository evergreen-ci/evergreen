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

func orphanedVersionCleanupGenerator(env anser.Environment, db string, limit int) (anser.Generator, error) {
	const migrationName = "clean_orphaned_versions"

	if err := env.RegisterManualMigrationOperation(migrationName, orphanedVersionCleanup); err != nil {
		return nil, err
	}

	opts := model.GeneratorOptions{
		NS: model.Namespace{
			DB:         db,
			Collection: version.Collection,
		},
		Query: bson.M{
			version.RequesterKey: evergreen.RepotrackerVersionRequester,
			version.StatusKey:    evergreen.VersionCreated, // TODO is this true?
		},
		Limit: limit,
		JobID: "migration-versions-orphans",
	}

	return anser.NewManualMigrationGenerator(env, opts, migrationName), nil
}

func orphanedVersionCleanup(session db.Session, rawD bson.RawD) error {
	defer session.Close()

	bstatuses := []version.BuildStatus{}
	builds := []string{}
	versionID := ""

	for i := range rawD {
		switch rawD[i].Name {
		case version.IdKey:
			if err := rawD[i].Value.Unmarshal(&versionID); err != nil {
				return errors.Wrap(err, "error unmarshaling id")
			}

		case version.BuildVariantsKey:
			if err := rawD[i].Value.Unmarshal(&bstatuses); err != nil {
				return errors.Wrap(err, "error unmarshaling build statuses")
			}

		case version.BuildIdsKey:
			if err := rawD[i].Value.Unmarshal(&builds); err != nil {
				return errors.Wrap(err, "error unmarshaling builds")
			}
		}
	}

	buildMap := map[string]*build.Build{}

	newBuilds := []string{}
	for _, buildID := range builds {
		b, ok := buildMap[buildID]
		if !ok {
			var err error
			b, err = build.FindOne(build.ById(buildID))
			if err != nil {
				return errors.Wrapf(err, "error fetching build %s", buildID)
			}
			if b != nil {
				buildMap[buildID] = b
			}
		}

		if b != nil {
			newBuilds = append(newBuilds, buildID)
		}
	}

	newBuildStatuses := []version.BuildStatus{}
	for _, bstatus := range bstatuses {
		b, ok := buildMap[bstatus.BuildId]
		if !ok {
			var err error
			b, err = build.FindOne(build.ById(bstatus.BuildId))
			if err != nil {
				return errors.Wrapf(err, "error fetching build %s", bstatus.BuildId)
			}

			if b != nil {
				buildMap[bstatus.BuildId] = b
			}
		}

		if b != nil {
			newBuildStatuses = append(newBuildStatuses, bstatus)
		}
	}

	selector := bson.M{
		version.IdKey: versionID,
	}
	if len(newBuilds) == 0 && len(newBuildStatuses) == 0 {
		return version.RemoveOne(selector)
	}

	update := bson.M{
		"$set": bson.M{
			version.BuildIdsKey:      newBuilds,
			version.BuildVariantsKey: newBuildStatuses,
		},
	}

	return version.UpdateOne(selector, update)
}

func orphanedBuildCleanupGenerator(env anser.Environment, db string, limit int) (anser.Generator, error) {
	const migrationName = "clean_orphaned_builds"

	if err := env.RegisterManualMigrationOperation(migrationName, makeOrphanedBuildCleanup(db)); err != nil {
		return nil, err
	}

	opts := model.GeneratorOptions{
		NS: model.Namespace{
			DB:         db,
			Collection: build.Collection,
		},
		Query: bson.M{
			build.RequesterKey: evergreen.RepotrackerVersionRequester,
			build.StatusKey:    evergreen.BuildCreated, // TODO is this true?
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
	return func(session db.Session, rawD bson.RawD) error {
		defer session.Close()

		cachedTasks := []build.TaskCache{}
		versionID := ""
		buildID := ""

		for i := range rawD {
			switch rawD[i].Name {
			case build.IdKey:
				if err := rawD[i].Value.Unmarshal(&buildID); err != nil {
					return errors.Wrap(err, "error unmarshaling id")
				}

			case build.TasksKey:
				if err := rawD[i].Value.Unmarshal(&cachedTasks); err != nil {
					return errors.Wrap(err, "error unmarshaling tasks")
				}

			case build.VersionKey:
				if err := rawD[i].Value.Unmarshal(&versionID); err != nil {
					return errors.Wrap(err, "error unmarshaling version")
				}
			}
		}

		query := session.DB(database).C(version.Collection).FindId(versionID)
		v := version.Version{}
		err := query.One(&v)

		if err == mgo.ErrNotFound {
			if err := session.DB(database).C(task.Collection).Remove(bson.M{task.BuildIdKey: buildID}); err != nil {
				return errors.Wrap(err, "error deleting tasks for build")
			}

			return session.DB(database).C(build.Collection).Remove(bson.M{build.IdKey: buildID})
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
		query = session.DB(database).C(task.Collection).Find(bson.M{
			task.IdKey: bson.M{
				"$in": cachedTaskIDs,
			},
		})
		if err := query.All(&tasks); err != nil {
			return err
		}
		if len(tasks) == 0 {
			return session.DB(database).C(build.Collection).Remove(bson.M{build.IdKey: buildID})
		}
		if len(tasks) == len(cachedTasks) {
			return nil
		}

		newTasksCache := []build.TaskCache{}
		// TODO?: this does not deal with tasks that weren't listed in
		// the task cache, but were found by the query.
		for i := range tasks {
			if tc, ok := cachedTasksMap[tasks[i].Id]; ok {
				newTasksCache = append(newTasksCache, *tc)
			}
		}

		update := bson.M{
			"$set": bson.M{
				build.TasksKey: newTasksCache,
			},
		}

		return session.DB(database).C(build.Collection).Update(bson.M{build.IdKey: buildID}, update)
	}
}
