package anser

import (
	"fmt"
	"sync"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"gopkg.in/mgo.v2/bson"
)

func init() {
	registry.AddJobType("manual-migration-generator",
		func() amboy.Job { return makeManualGenerator() })
}

func NewManualMigrationGenerator(e Environment, opts model.GeneratorOptions, opName string) Generator {
	j := makeManualGenerator()
	j.SetDependency(generatorDependency(opts))
	j.SetID(opts.JobID)
	j.MigrationHelper = NewMigrationHelper(e)
	j.NS = opts.NS
	j.Query = opts.Query
	j.OperationName = opName

	return j
}

func makeManualGenerator() *manualMigrationGenerator {
	return &manualMigrationGenerator{
		MigrationHelper: &migrationBase{},
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    "manual-migration-generator",
				Version: 0,
				Format:  amboy.BSON,
			},
		},
	}
}

type manualMigrationGenerator struct {
	NS              model.Namespace        `bson:"ns" json:"ns" yaml:"ns"`
	Query           map[string]interface{} `bson:"source_query" json:"source_query" yaml:"source_query"`
	OperationName   string                 `bson:"op_name" json:"op_name" yaml:"op_name"`
	Migrations      []*manualMigrationJob  `bson:"migrations" json:"migrations" yaml:"migrations"`
	job.Base        `bson:"job_base" json:"job_base" yaml:"job_base"`
	MigrationHelper `bson:"-" json:"-" yaml:"-"`
	mu              sync.Mutex
}

func (j *manualMigrationGenerator) Run() {
	defer j.MarkComplete()

	env := j.Env()

	network, err := env.GetDependencyNetwork()
	if err != nil {
		j.AddError(err)
		return
	}

	session, err := env.GetSession()
	if err != nil {
		j.AddError(err)
		return
	}
	defer session.Close()

	coll := session.DB(j.NS.DB).C(j.NS.Collection)
	iter := coll.Find(j.Query).Select(bson.M{"_id": 1}).Iter()

	network.AddGroup(j.ID(), j.generateJobs(env, iter))

	if err := iter.Close(); err != nil {
		j.AddError(err)
		return
	}
}

func (j *manualMigrationGenerator) generateJobs(env Environment, iter db.Iterator) []string {
	ids := []string{}
	doc := struct {
		ID interface{} `bson:"_id"`
	}{}

	j.mu.Lock()
	defer j.mu.Unlock()
	for iter.Next(&doc) {
		m := NewManualMigration(env, model.Manual{
			ID:            doc.ID,
			OperationName: j.OperationName,
			Migration:     j.ID(),
			Namespace:     j.NS,
		}).(*manualMigrationJob)

		m.SetDependency(env.NewDependencyManager(j.ID(), j.Query, j.NS))
		m.SetID(fmt.Sprintf("%s.%v.%d", j.ID(), doc.ID, len(ids)))
		ids = append(ids, m.ID())
		j.Migrations = append(j.Migrations, m)
	}

	return ids
}

func (j *manualMigrationGenerator) Jobs() <-chan amboy.Job {
	env := j.Env()

	j.mu.Lock()
	defer j.mu.Unlock()

	jobs := make(chan amboy.Job, len(j.Migrations))
	for _, job := range j.Migrations {
		jobs <- job
	}
	close(jobs)

	out, err := generator(env, j.ID(), jobs)
	grip.CatchError(err)
	grip.Infof("produced %d tasks for migration %s", len(j.Migrations), j.ID())
	j.Migrations = []*manualMigrationJob{}
	return out
}
