package anser

import (
	"context"
	"testing"
	"time"

	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/anser/client"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	mgo "gopkg.in/mgo.v2"
)

// proofOfConcept is a simple mock "main" to demonstrate how you could
// build a simple migration utility.
func proofOfConcept() error {
	env := GetEnvironment()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := queue.NewAdaptiveOrderedLocalQueue(3, 3)

	if err := q.Start(ctx); err != nil {
		return err
	}

	ses, err := mgo.DialWithTimeout("mongodb://localhost:27017", 10*time.Millisecond)
	if err != nil {
		return err
	}
	session := db.WrapSession(ses)

	cl, err := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017").SetConnectTimeout(10 * time.Millisecond))
	if err != nil {
		return err
	}

	client := client.WrapClient(cl)
	if err := env.Setup(q, client, session); err != nil {
		return err
	}

	env.SetPreferedDB(client)

	ns := model.Namespace{DB: "mci", Collection: "test"}

	app := &Application{
		Generators: []Generator{
			NewSimpleMigrationGenerator(env,
				model.GeneratorOptions{
					JobID:     "first",
					DependsOn: []string{},
					NS:        ns,
					Query: map[string]interface{}{
						"time": map[string]interface{}{"$gt": time.Now().Add(-time.Hour)},
					},
				},
				// update:
				map[string]interface{}{
					"$rename": map[string]string{"time": "timeSince"},
				}),
			NewStreamMigrationGenerator(env,
				model.GeneratorOptions{
					JobID:     "second",
					DependsOn: []string{"first"},
					NS:        ns,
					Query: map[string]interface{}{
						"time": map[string]interface{}{"$gt": time.Now().Add(-time.Hour)},
					},
				},
				// the name of a registered aggregate operation
				"op-name"),
		},
	}

	if err := app.Setup(env); err != nil {
		return err
	}

	return app.Run(ctx)
}

func TestExampleApp(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(proofOfConcept())
}
