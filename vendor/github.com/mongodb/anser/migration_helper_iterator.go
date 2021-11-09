package anser

import (
	"context"
	"sync"

	"github.com/mongodb/amboy/job"
	"github.com/mongodb/anser/client"
	"github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type cursorMigrationMetadataIterator struct {
	cursor  client.Cursor
	catcher grip.Catcher
	ctx     context.Context
}

func (c *cursorMigrationMetadataIterator) Err() error   { return c.catcher.Resolve() }
func (c *cursorMigrationMetadataIterator) Close() error { return c.cursor.Close(context.TODO()) }
func (c *cursorMigrationMetadataIterator) Next(ctx context.Context) bool {
	ret := c.cursor.Next(ctx)
	c.catcher.AddWhen(!ret, c.cursor.Err())
	return ret
}

func (c *cursorMigrationMetadataIterator) Item() *model.MigrationMetadata {
	o := &model.MigrationMetadata{}
	c.catcher.Add(c.cursor.Decode(o))
	return o
}

func NewClientMigrationHelper(e Environment) MigrationHelper { return &migrationBase{env: e} }

type migrationBase struct {
	env Environment
	sync.Mutex
}

func (m *migrationBase) Env() Environment {
	m.Lock()
	defer m.Unlock()

	if m.env == nil {
		m.env = GetEnvironment()
	}

	return m.env

}

func (m *migrationBase) SaveMigrationEvent(ctx context.Context, mm *model.MigrationMetadata) error {
	env := m.Env()

	client, err := env.GetClient()
	if err != nil {
		return errors.WithStack(err)
	}
	ns := env.MetadataNamespace()

	res, err := client.Database(ns.DB).Collection(ns.Collection).ReplaceOne(ctx,
		bson.M{"_id": mm.ID},
		mm, options.Replace().SetUpsert(true))

	if err != nil {
		return errors.WithStack(err)
	}

	if res.MatchedCount+res.UpsertedCount+res.ModifiedCount < 1 {
		return errors.Errorf("migration event was not saved for '%s'", mm.ID)
	}

	return nil
}

func (m *migrationBase) FinishMigration(ctx context.Context, name string, j *job.Base) {
	j.MarkComplete()
	meta := model.MigrationMetadata{
		ID:        j.ID(),
		Migration: name,
		HasErrors: j.HasErrors(),
		Completed: true,
	}
	err := m.SaveMigrationEvent(ctx, &meta)
	if err != nil {
		j.AddError(err)
		grip.Warning(message.Fields{
			"message": "encountered problem saving migration event",
			"id":      j.ID(),
			"name":    name,
			"error":   err.Error(),
			"type":    "client",
		})
		return
	}

	grip.Debug(message.Fields{
		"message":  "completed migration",
		"id":       j.ID(),
		"name":     name,
		"metadata": meta,
	})
}

func (m *migrationBase) PendingMigrationOperations(ctx context.Context, ns model.Namespace, q map[string]interface{}) int {
	return 0
}

func (m *migrationBase) GetMigrationEvents(ctx context.Context, q map[string]interface{}) MigrationMetadataIterator {
	env := m.Env()

	client, err := env.GetClient()
	if err != nil {
		return &errorMigrationIterator{err: err}
	}

	ns := env.MetadataNamespace()
	cursor, err := client.Database(ns.DB).Collection(ns.Collection).Find(ctx, q)
	if err != nil {
		return &errorMigrationIterator{err: err}
	}

	if cursor == nil {
		return &errorMigrationIterator{}

	}
	return &cursorMigrationMetadataIterator{ctx: ctx, cursor: cursor, catcher: grip.NewBasicCatcher()}
}
