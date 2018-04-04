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
	"gopkg.in/mgo.v2/bson"
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

func (j *simpleMigrationJob) Run(_ context.Context) {
	grip.Info(message.Fields{
		"message":   "starting migration",
		"operation": "simple",
		"migration": j.Definition.Migration,
		"target":    j.Definition.ID,
		"id":        j.ID(),
		"ns":        j.Definition.Namespace,
	})

	defer j.FinishMigration(j.Definition.Migration, &j.Base)

	env := j.Env()
	session, err := env.GetSession()
	if err != nil {
		j.AddError(errors.Wrap(err, "problem getting database session"))
		return
	}
	defer session.Close()

	coll := session.DB(j.Definition.Namespace.DB).C(j.Definition.Namespace.Collection)

	j.AddError(coll.UpdateId(j.Definition.ID, bson.M(j.Definition.Update)))
}
