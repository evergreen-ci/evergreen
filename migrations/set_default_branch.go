package migrations

import (
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/model"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	migrationSetDefaultBranch = "project-set-default-branch"
)

func setDefaultBranchMigrationGenerator(env anser.Environment, args migrationGeneratorFactoryOptions) (anser.Generator, error) {
	const (
		branchKey  = "branch_name"
		collection = "project_ref"
	)
	opts := model.GeneratorOptions{
		NS: model.Namespace{
			DB:         args.db,
			Collection: collection,
		},
		Limit: args.limit,
		Query: bson.M{
			branchKey: "",
		},
		JobID: args.id,
	}

	return anser.NewSimpleMigrationGenerator(env, opts, bson.M{
		"$set": bson.M{
			branchKey: "master",
		},
	}), nil
}
