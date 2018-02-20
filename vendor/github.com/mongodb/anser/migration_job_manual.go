package anser

import (
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

func (j *manualMigrationJob) Run() {
	grip.Info(message.Fields{
		"message":   "starting migration",
		"operation": "manual",
		"migration": j.Definition.Migration,
		"target":    j.Definition.ID,
		"id":        j.ID(),
		"ns":        j.Definition.Namespace,
	})

	defer j.FinishMigration(j.Definition.Migration, &j.Base)
	env := j.Env()

	operation, ok := env.GetManualMigrationOperation(j.Definition.OperationName)
	if !ok {
		j.AddError(errors.Errorf("could not find migration operation named %s", j.Definition.OperationName))
		return
	}

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
}
