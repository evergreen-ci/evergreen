package migrations

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/go-github/github"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

func zeroDateFixGenerator(githubToken string) migrationGeneratorFactory {
	return func(env anser.Environment, db string, limit int) (anser.Generator, error) {
		const (
			versionCollection = "versions"
			createTimeKey     = "create_time"

			migrationName = "zero-date-fix"
		)

		if err := env.RegisterManualMigrationOperation(migrationName, makeZeroDateMigration(db, githubToken)); err != nil {
			return nil, err
		}

		loc, err := time.LoadLocation("UTC")
		if err != nil {
			return nil, err
		}

		// 19:03 == 00:03 UTC, but zero time is 00:00 UTC
		// I have no idea where the extra 3 minutes came from in
		// EVG, so we add 4 minutes here to make sure we capture them
		minTime := time.Time{}.In(loc).Add(4 * time.Minute)

		opts := model.GeneratorOptions{
			NS: model.Namespace{
				DB:         db,
				Collection: versionCollection,
			},
			Limit: limit,
			Query: bson.M{
				createTimeKey: bson.M{
					"$lte": minTime,
				},
			},
			JobID: "migration-zero-date-fix",
		}

		return anser.NewManualMigrationGenerator(env, opts, migrationName), nil
	}

}

func makeZeroDateMigration(database, githubToken string) db.MigrationOperation {
	const (
		versionCollection    = "versions"
		projectRefCollection = "project_ref"

		createTimeKey = "create_time"
		revisionKey   = "gitspec"
		projectIDKey  = "identifier"
		idKey         = "_id"

		ownerKey = "owner_name"
		repoKey  = "repo_name"
	)

	return func(session db.Session, rawD bson.RawD) error {
		defer session.Close()

		revision := ""
		versionID := ""
		projectID := ""
		for _, raw := range rawD {
			switch raw.Name {
			case idKey:
				if err := raw.Value.Unmarshal(&versionID); err != nil {
					return errors.Wrap(err, "error unmarshalling version ID")
				}

			case revisionKey:
				if err := raw.Value.Unmarshal(&revision); err != nil {
					return errors.Wrap(err, "error unmarshalling hash")
				}

			case projectIDKey:
				if err := raw.Value.Unmarshal(&projectID); err != nil {
					return errors.Wrap(err, "error unmarshalling project identifier")
				}
			}
		}
		if revision == "" || versionID == "" || projectID == "" {
			return errors.New("revision or versionID or projectID was empty")
		}

		// find project ref with identifier
		query := session.DB(database).C(projectRefCollection).Find(bson.M{
			projectIDKey: projectID,
		})
		out := bson.M{}
		if err := query.One(&out); err != nil {
			return errors.Wrapf(err, "can't find project ref: %s", projectID)
		}

		owner, ok := out[ownerKey].(string)
		if !ok {
			return errors.New("owner was not a string")
		}
		repo, ok := out[repoKey].(string)
		if !ok {
			return errors.New("repo was not a string")
		}

		if owner == "" || repo == "" {
			return errors.New("owner/repo are empty")
		}

		newTime, err := githubFetchRealCreateTime(githubToken, owner, repo, revision)
		if err != nil {
			return errors.WithStack(err)
		}
		if err = updateBuildCreateTime(session.DB(database), versionID, *newTime); err != nil {
			return errors.WithStack(err)
		}
		if err = updateTaskCreateTime(session.DB(database), versionID, *newTime); err != nil {
			return errors.WithStack(err)
		}
		if err = updatePatchCreateTime(session.DB(database), versionID, *newTime); err != nil {
			return errors.WithStack(err)
		}

		return session.DB(database).C(versionCollection).UpdateId(versionID,
			bson.M{
				"$set": bson.M{
					createTimeKey: *newTime,
				},
			})
	}
}

func githubFetchRealCreateTime(token, owner, repo, revision string) (*time.Time, error) {
	ctx := context.Background()
	httpClient, err := util.GetHttpClientForOauth2(token)
	if err != nil {
		return nil, err
	}
	defer util.PutHttpClientForOauth2(httpClient)

	client := github.NewClient(httpClient)

	commit, _, err := client.Git.GetCommit(ctx, owner, repo, revision)
	if err != nil {
		return nil, errors.Wrap(err, "can't fetch commit info from github")
	}
	if commit == nil {
		return nil, errors.Errorf("Couldn't fetch commit on %s/%s for hash: %s", owner, repo, revision)
	}

	if commit.Committer == nil || commit.Committer.Date == nil {
		return nil, errors.Errorf("Empty data returned while fetching commit on %s/%s for hash: %s", owner, repo, revision)
	}

	return commit.Committer.Date, nil
}

func updateBuildCreateTime(dbs db.Database, versionID string, newTime time.Time) error {
	const (
		buildCollection = "builds"

		idKey         = "_id"
		versionKey    = "version"
		createTimeKey = "create_time"
	)

	change, err := dbs.C(buildCollection).UpdateAll(bson.M{
		versionKey: versionID,
	}, bson.M{
		"$set": bson.M{
			createTimeKey: newTime,
		},
	})
	if err != nil {
		return errors.WithStack(err)
	}
	msg := message.Fields{
		"message":  "updated builds",
		"version":  versionID,
		"new_time": newTime.String(),
	}

	if change != nil {
		msg["updated"] = change.Updated
	}
	grip.Info(msg)

	return nil
}

func updatePatchCreateTime(dbs db.Database, versionID string, newTime time.Time) error {
	const (
		tasksCollection = "tasks"

		idKey         = "_id"
		versionKey    = "version"
		createTimeKey = "create_time"
	)

	change, err := dbs.C(tasksCollection).UpdateAll(bson.M{
		versionKey: versionID,
	}, bson.M{
		"$set": bson.M{
			createTimeKey: newTime,
		},
	})
	if err != nil {
		return errors.WithStack(err)
	}

	msg := message.Fields{
		"message":  "updated tasks (most recent execution only)",
		"version":  versionID,
		"new_time": newTime.String(),
	}
	if change != nil {
		msg["updated"] = change.Updated
	}
	grip.Info(msg)

	return nil
}

func updateTaskCreateTime(dbs db.Database, versionID string, newTime time.Time) error {
	const (
		patchesCollection = "patches"

		idKey         = "_id"
		versionKey    = "version"
		createTimeKey = "create_time"
	)

	change, err := dbs.C(patchesCollection).UpdateAll(bson.M{
		versionKey: versionID,
	}, bson.M{
		"$set": bson.M{
			createTimeKey: newTime,
		},
	})
	if err != nil {
		return errors.WithStack(err)
	}

	msg := message.Fields{
		"message":  "updated patches",
		"version":  versionID,
		"new_time": newTime.String(),
	}
	if change != nil {
		msg["updated"] = change.Updated
	}
	grip.Info(msg)

	return nil
}
