package migrations

import (
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

func githubHooksToCollectionGenerator(env anser.Environment, db string, limit int) (anser.Generator, error) {
	const (
		projectVarsCollection = "project_vars"
		migrationName         = "github_hooks_to_collection_generator"
		githubHookIDKey       = "github_hook_id"
	)

	if err := env.RegisterManualMigrationOperation(migrationName, makeGithubHooksMigration(db)); err != nil {
		return nil, err
	}

	opts := model.GeneratorOptions{
		NS: model.Namespace{
			DB:         db,
			Collection: projectVarsCollection,
		},
		Limit: limit,
		Query: bson.M{
			githubHookIDKey: bson.M{
				"$exists": true,
				"$ne":     0,
			},
		},
		JobID: "migration-github-hooks-to-collection",
	}

	return anser.NewManualMigrationGenerator(env, opts, migrationName), nil
}

func makeGithubHooksMigration(database string) db.MigrationOperation {
	const (
		projectRefCollection  = "project_ref"
		projectVarsCollection = "project_vars"
		githubHooksCollection = "github_hooks"

		idKey = "_id"

		// project_vars
		githubHookIDKey = "github_hook_id"

		// project ref
		ownerKey      = "owner"
		repoKey       = "repo"
		identifierKey = "identifier"
	)

	return func(session db.Session, rawD bson.RawD) error {
		defer session.Close()

		projectVarsID := ""
		hookID := 0
		for _, raw := range rawD {
			switch raw.Name {
			case idKey:
				if err := raw.Value.Unmarshal(&projectVarsID); err != nil {
					return errors.Wrap(err, "error unmarshaling id")
				}

			case githubHookIDKey:
				if err := raw.Value.Unmarshal(&hookID); err != nil {
					return errors.Wrap(err, "error unmarshaling github Hook ID")
				}
			}
		}

		// find project ref with identifier
		query := session.DB(database).C(projectRefCollection).Find(bson.M{
			identifierKey: projectVarsID,
		})
		ref := struct {
			Owner string `bson:"owner_name"`
			Repo  string `bson:"repo_name"`
		}{}
		err := query.One(&ref)
		if err != nil {
			return err
		}

		// check if there is an existing github hook for the same repo
		query = session.DB(database).C(githubHooksCollection).Find(bson.M{
			ownerKey: ref.Owner,
			repoKey:  ref.Repo,
		})
		existingHook := struct {
			HookID int    `bson:"_id"`
			Owner  string `bson:"owner"`
			Repo   string `bson:"repo"`
		}{}
		err = query.One(&existingHook)
		if err != nil && !db.ResultsNotFound(err) {
			return err
		}

		if existingHook.HookID != 0 {
			// Not calling this an error, since it's technically done now.
			grip.Infof("Hook for %s/%s already exists as Hook #%d. Please delete %d",
				existingHook.Owner, existingHook.Repo, existingHook.HookID, hookID)
			return nil
		}

		// insert the new hook
		hook := bson.M{
			idKey:    hookID,
			ownerKey: ref.Owner,
			repoKey:  ref.Repo,
		}

		err = session.DB(database).C(githubHooksCollection).Insert(hook)
		if err != nil {
			return err
		}

		return session.DB(database).C(projectVarsCollection).UpdateId(projectVarsID,
			bson.M{
				"$unset": bson.M{
					githubHookIDKey: 1,
				},
			})
	}
}
