package migrations

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
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

func orphanedTaskCleanupGenerator(env anser.Environment, db string, limit int) (anser.Generator, error) {
	const migrationName = "clean_orphaned_tasks"

	if err := env.RegisterManualMigrationOperation(migrationName, makeOrphanedTaskCleanup(db)); err != nil {
		return nil, err
	}

	opts := model.GeneratorOptions{
		NS: model.Namespace{
			DB:         db,
			Collection: taskCollection,
		},
		Query: bson.M{
			requesterKey: evergreen.RepotrackerVersionRequester,
		},
		Limit: limit,
		JobID: "migration-builds-tasks",
	}

	return anser.NewManualMigrationGenerator(env, opts, migrationName), nil
}

// orpganedTaskCleanup, given a task, will delete it if the build or version
// it claims to belong to doesn't exist
func makeOrphanedTaskCleanup(database string) db.MigrationOperation {
	const (
		idKey      = "_id"
		tasksKey   = "tasks"
		versionKey = "version"
		buildIDKey = "build_id"
	)
	return func(session db.Session, rawD bson.RawD) error {
		defer session.Close()

		versionID := ""
		buildID := ""
		taskID := ""

		for i := range rawD {
			switch rawD[i].Name {
			case idKey:
				if err := rawD[i].Value.Unmarshal(&taskID); err != nil {
					return errors.Wrap(err, "error unmarshaling id")
				}

			case buildIDKey:
				if err := rawD[i].Value.Unmarshal(&buildID); err != nil {
					return errors.Wrap(err, "error unmarshaling tasks")
				}

			case versionKey:
				if err := rawD[i].Value.Unmarshal(&versionID); err != nil {
					return errors.Wrap(err, "error unmarshaling version")
				}
			}
		}

		queryVersion := session.DB(database).C(versionCollection).FindId(versionID)
		queryBuild := session.DB(database).C(buildCollection).FindId(buildID)
		errV := queryVersion.One(nil)
		errB := queryBuild.One(nil)

		if errV == mgo.ErrNotFound || errB == mgo.ErrNotFound {
			return errors.WithStack(session.DB(database).C(taskCollection).RemoveId(taskID))
		}
		if errV != nil {
			return errors.WithStack(errV)
		}
		if errB != nil {
			return errors.WithStack(errB)
		}

		return nil
	}
}

func duplicateVersionsCleanup(env anser.Environment, db string, limit int) (anser.Generator, error) {
	const migrationName = "clean_duplicate_versions"

	if err := env.RegisterManualMigrationOperation(migrationName, makeDuplicateVersionCleanup(db)); err != nil {
		return nil, err
	}

	opts := model.GeneratorOptions{
		NS: model.Namespace{
			DB:         db,
			Collection: versionCollection,
		},
		Query: bson.M{
			requesterKey: evergreen.RepotrackerVersionRequester,
		},
		Limit: limit,
		JobID: "migration-duplicate-versions",
	}

	return anser.NewManualMigrationGenerator(env, opts, migrationName), nil
}

func makeDuplicateVersionCleanup(database string) db.MigrationOperation {
	const (
		idKey           = "_id"
		hashKey         = "gitspec"
		projectRefIDKey = "identifier"
		versionKey      = "version"
	)
	return func(session db.Session, rawD bson.RawD) error {
		defer session.Close()

		versionID := ""
		projectID := ""
		hash := ""

		for i := range rawD {
			switch rawD[i].Name {
			case idKey:
				if err := rawD[i].Value.Unmarshal(&versionID); err != nil {
					return errors.Wrap(err, "error unmarshaling id")
				}

			case projectRefIDKey:
				if err := rawD[i].Value.Unmarshal(&projectID); err != nil {
					return errors.Wrap(err, "error unmarshaling project ID")
				}

			case hashKey:
				if err := rawD[i].Value.Unmarshal(&hash); err != nil {
					return errors.Wrap(err, "error unmarshaling git hash")
				}
			}
		}

		versions := []version.Version{}
		query := session.DB(database).C(versionCollection).Find(bson.M{
			requesterKey:    evergreen.RepotrackerVersionRequester,
			hashKey:         hash,
			projectRefIDKey: projectID,
		})
		err := query.All(&versions)
		if err != nil {
			return errors.WithStack(err)
		}
		if len(versions) == 1 {
			return nil
		}

		deleteThese := findVersionsToDelete(versions)

		catcher := grip.NewSimpleCatcher()
		for _, id := range deleteThese {
			catcher.Add(session.DB(database).C(versionCollection).RemoveId(id))
			_, err := session.DB(database).C(buildCollection).RemoveAll(bson.M{
				versionKey: id,
			})
			if err == mgo.ErrNotFound {
				err = nil
			}
			catcher.Add(errors.Wrapf(err, "trying to delete builds with version %s", versionID))
			_, err = session.DB(database).C(taskCollection).RemoveAll(bson.M{
				versionKey: id,
			})
			if err == mgo.ErrNotFound {
				err = nil
			}
			catcher.Add(errors.Wrapf(err, "trying to delete tasks with version %s", versionID))
		}

		return catcher.Resolve()
	}
}

func findVersionsToDelete(v []version.Version) []string {
	mostProgress := &v[0]

	for i := range v {
		if rankVersionStatus(v[i].Status) > rankVersionStatus(mostProgress.Status) {
			mostProgress = &v[i]
		}
	}

	deleteThese := []string{}
	for i := range v {
		if mostProgress.Id == v[i].Id {
			continue
		}

		deleteThese = append(deleteThese, v[i].Id)
	}

	return deleteThese
}

func rankVersionStatus(s string) int {
	switch s {
	case evergreen.VersionSucceeded, evergreen.VersionFailed:
		return 2
	case evergreen.VersionStarted:
		return 1
	case evergreen.VersionCreated:
		return 0
	default:
		return -100
	}
}
