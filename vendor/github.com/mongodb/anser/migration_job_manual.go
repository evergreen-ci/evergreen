package anser

import (
	"context"

	"github.com/evergreen-ci/birch"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

func init() {
	registry.AddJobType("manual-migration", func() amboy.Job { return makeManualMigration() })
}

func NewManualMigration(e Environment, m model.Manual) Migration {
	j := makeManualMigration()
	j.Definition = m
	j.MigrationHelper = NewMigrationHelper(e)

	return j
}

func makeManualMigration() *manualMigrationJob {
	return &manualMigrationJob{
		MigrationHelper: &migrationBase{},
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    "manual-migration",
				Version: 0,
			},
		},
	}
}

type manualMigrationJob struct {
	Definition      model.Manual `bson:"migration" json:"migration" yaml:"migration"`
	job.Base        `bson:"job_base" json:"job_base" yaml:"job_base"`
	MigrationHelper `bson:"-" json:"-" yaml:"-"`
}

func (j *manualMigrationJob) Run(ctx context.Context) {
	grip.Info(message.Fields{
		"message":   "starting migration",
		"operation": "manual",
		"migration": j.Definition.Migration,
		"target":    j.Definition.ID,
		"id":        j.ID(),
		"ns":        j.Definition.Namespace,
		"name":      j.Definition.OperationName,
	})

	defer j.FinishMigration(ctx, j.Definition.Migration, &j.Base)
	env := j.Env()

	if operation, ok := env.GetLegacyManualMigrationOperation(j.Definition.OperationName); ok {
		session, err := env.GetSession()
		if err != nil {
			j.AddError(errors.Wrap(err, "problem getting database session"))
			return
		}
		defer session.Close()

		var doc bson.RawD
		coll := session.DB(j.Definition.Namespace.DB).C(j.Definition.Namespace.Collection)
		err = coll.FindId(j.Definition.ID).One(&doc)
		if err != nil {
			j.AddError(err)
			return
		}

		j.AddError(operation(session, doc))
	} else if operation, ok := env.GetManualMigrationOperation(j.Definition.OperationName); ok {
		client, err := env.GetClient()
		if err != nil {
			j.AddError(errors.Wrap(err, "problem getting database client"))
			return
		}

		coll := client.Database(j.Definition.Namespace.DB).Collection(j.Definition.Namespace.Collection)

		res := coll.FindOne(ctx, bson.M{"_id": j.Definition.ID})
		if err := res.Err(); err != nil {
			j.AddError(err)
			return
		}
		payload, err := res.DecodeBytes()
		if err != nil {
			j.AddError(err)
			return
		}

		doc, err := birch.ReadDocument(payload)
		if err != nil {
			j.AddError(err)
			return
		}

		j.AddError(operation(client, doc))
	} else {
		j.AddError(errors.Errorf("could not find migration named '%s'", j.Definition.OperationName))
	}
}
