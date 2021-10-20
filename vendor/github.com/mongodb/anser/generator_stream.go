package anser

import (
	"context"
	"fmt"
	"sync"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/anser/client"
	"github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func init() {
	registry.AddJobType("stream-migration-generator",
		func() amboy.Job { return makeStreamGenerator() })
}

func NewStreamMigrationGenerator(e Environment, opts model.GeneratorOptions, opName string) Generator {
	j := makeStreamGenerator()
	j.SetID(opts.JobID)
	j.SetDependency(generatorDependency(e, opts))
	j.MigrationHelper = NewMigrationHelper(e)
	j.NS = opts.NS
	j.Query = opts.Query
	j.ProcessorName = opName
	j.Limit = opts.Limit
	return j
}

func makeStreamGenerator() *streamMigrationGenerator {
	return &streamMigrationGenerator{
		MigrationHelper: &migrationBase{},
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    "stream-migration-generator",
				Version: 0,
			},
		},
	}
}

type streamMigrationGenerator struct {
	NS              model.Namespace        `bson:"ns" json:"ns" yaml:"ns"`
	Query           map[string]interface{} `bson:"source_query" json:"source_query" yaml:"source_query"`
	Limit           int                    `bson:"limit" json:"limit" yaml:"limit"`
	ProcessorName   string                 `bson:"processor_name" json:"processor_name" yaml:"processor_name"`
	Migrations      []*streamMigrationJob  `bson:"migrations" json:"migrations" yaml:"migrations"`
	job.Base        `bson:"job_base" json:"job_base" yaml:"job_base"`
	MigrationHelper `bson:"-" json:"-" yaml:"-"`
	mu              sync.Mutex
}

func (j *streamMigrationGenerator) Run(ctx context.Context) {
	defer j.FinishMigration(ctx, j.ID(), &j.Base)

	env := j.Env()

	network, err := env.GetDependencyNetwork()
	if err != nil {
		j.AddError(err)
		return
	}

	client, err := env.GetClient()
	if err != nil {
		j.AddError(err)
		return
	}

	findOpts := options.Find().SetProjection(bson.M{"_id": 1})
	if j.Limit > 0 {
		findOpts.SetLimit(int64(j.Limit))
	}

	cursor, err := client.Database(j.NS.DB).Collection(j.NS.Collection).Find(ctx, j.Query, findOpts) // findOpts
	if err != nil {
		j.AddError(err)
		return
	}

	network.AddGroup(j.ID(), j.generateJobs(ctx, env, cursor))
}

func (j *streamMigrationGenerator) generateJobs(ctx context.Context, env Environment, iter client.Cursor) []string {
	ids := []string{}
	doc := struct {
		ID interface{} `bson:"_id"`
	}{}

	j.mu.Lock()
	defer j.mu.Unlock()
	count := 0

	for iter.Next(ctx) {
		count++
		if err := iter.Decode(&doc); err != nil {
			grip.Error(message.WrapError(err, "problem decoding generator results"))
			break
		}

		m := NewStreamMigration(env, model.Stream{
			ProcessorName: j.ProcessorName,
			Migration:     j.ID(),
			Namespace:     j.NS,
			Query:         j.Query,
		}).(*streamMigrationJob)

		m.SetDependency(env.NewDependencyManager(j.ID()))
		m.SetID(fmt.Sprintf("%s.%v.%d", j.ID(), doc.ID, len(ids)))
		ids = append(ids, m.ID())
		j.Migrations = append(j.Migrations, m)

		grip.Debug(message.Fields{
			"ns":  j.NS,
			"id":  m.ID(),
			"doc": doc.ID,
			"num": count,
		})

		if j.Limit > 0 && count >= j.Limit {
			break
		}
	}

	return ids
}

func (j *streamMigrationGenerator) Jobs() <-chan amboy.Job {
	env := j.Env()

	j.mu.Lock()
	defer j.mu.Unlock()

	jobs := make(chan amboy.Job, len(j.Migrations))
	for _, job := range j.Migrations {
		jobs <- job
	}
	close(jobs)

	out, err := generator(env, j.ID(), jobs)
	grip.Error(err)
	grip.Infof("produced %d tasks for migration %s", len(j.Migrations), j.ID())
	j.Migrations = []*streamMigrationJob{}
	return out
}
