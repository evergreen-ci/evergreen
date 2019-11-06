package anser

import (
	"context"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"
)

func init() {
	registry.AddJobType("simple-migration",
		func() amboy.Job { return makeSimpleMigration() })
}

func NewSimpleMigration(e Environment, m model.Simple) Migration {
	j := makeSimpleMigration()
	j.Definition = m
	j.MigrationHelper = NewMigrationHelper(e)
	return j
}

func makeSimpleMigration() *simpleMigrationJob {
	return &simpleMigrationJob{
		MigrationHelper: &migrationBase{},
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    "simple-migration",
				Version: 0,
			},
		},
	}
}

type simpleMigrationJob struct {
	Definition      model.Simple `bson:"migration" json:"migration" yaml:"migration"`
	job.Base        `bson:"job_base" json:"job_base" yaml:"job_base"`
	MigrationHelper `bson:"-" json:"-" yaml:"-"`
}

func (j *simpleMigrationJob) Run(ctx context.Context) {
	env := j.Env()
	preferClient := env.PreferClient()

	grip.Info(message.Fields{
		"message":   "starting migration",
		"operation": "simple",
		"migration": j.Definition.Migration,
		"target":    j.Definition.ID,
		"id":        j.ID(),
		"ns":        j.Definition.Namespace,
		"client":    preferClient,
	})

	defer j.FinishMigration(j.Definition.Migration, &j.Base)

	if preferClient {
		client, err := env.GetClient()
		if err != nil {
			j.AddError(errors.Wrap(err, "problem getting database client"))
			return
		}

		coll := client.Database(j.Definition.Namespace.DB).Collection(j.Definition.Namespace.Collection)
		res, err := coll.UpdateOne(ctx, bson.M{"_id": j.Definition.ID}, j.Definition.Update)
		j.AddError(err)
		if res.ModifiedCount != 1 {
			j.AddError(errors.Errorf("could not update '%s' for '%s'", j.Definition.ID, j.ID()))
		}
	} else {
		session, err := env.GetSession()
		if err != nil {
			j.AddError(errors.Wrap(err, "problem getting database session"))
			return
		}
		defer session.Close()

		coll := session.DB(j.Definition.Namespace.DB).C(j.Definition.Namespace.Collection)

		j.AddError(coll.UpdateId(j.Definition.ID, mgobson.M(j.Definition.Update)))
	}
}
