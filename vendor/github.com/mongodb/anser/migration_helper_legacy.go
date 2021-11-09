package anser

import (
	"context"
	"sync"

	"github.com/mongodb/amboy/job"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type errorMigrationIterator struct {
	err error
}

func (e *errorMigrationIterator) Err() error                     { return e.err }
func (e *errorMigrationIterator) Close() error                   { return nil }
func (e *errorMigrationIterator) Next(_ context.Context) bool    { return false }
func (e *errorMigrationIterator) Item() *model.MigrationMetadata { return nil }

type legacyMigrationMetadataIterator struct {
	iter db.Iterator
	item *model.MigrationMetadata
}

func (l *legacyMigrationMetadataIterator) Err() error   { return l.iter.Err() }
func (l *legacyMigrationMetadataIterator) Close() error { return l.iter.Close() }
func (l *legacyMigrationMetadataIterator) Next(ctx context.Context) bool {
	l.item = &model.MigrationMetadata{}
	return l.iter.Next(l.item)
}
func (l *legacyMigrationMetadataIterator) Item() *model.MigrationMetadata { return l.item }

// newLegacyMigrationHelper constructs a new legacy migration helper instance
// for internal testing. Use this to inject environment instances into tasks.
func newLegacyMigrationHelper(e Environment) MigrationHelper { return &legacyMigrationBase{env: e} }

type legacyMigrationBase struct {
	env Environment
	sync.Mutex
}

func (e *legacyMigrationBase) Env() Environment {
	e.Lock()
	defer e.Unlock()

	if e.env == nil {
		e.env = GetEnvironment()
	}

	return e.env
}

func (e *legacyMigrationBase) SaveMigrationEvent(ctx context.Context, m *model.MigrationMetadata) error {
	env := e.Env()

	session, err := env.GetSession()
	if err != nil {
		return errors.WithStack(err)
	}
	defer session.Close()

	ns := env.MetadataNamespace()
	coll := session.DB(ns.DB).C(ns.Collection)
	_, err = coll.UpsertId(m.ID, m)
	return errors.Wrap(err, "problem inserting migration metadata")
}

func (e *legacyMigrationBase) FinishMigration(ctx context.Context, name string, j *job.Base) {
	j.MarkComplete()
	meta := model.MigrationMetadata{
		ID:        j.ID(),
		Migration: name,
		HasErrors: j.HasErrors(),
		Completed: true,
	}
	err := e.SaveMigrationEvent(ctx, &meta)
	if err != nil {
		j.AddError(err)
		grip.Warning(message.Fields{
			"message": "encountered problem saving migration event",
			"id":      j.ID(),
			"name":    name,
			"error":   err.Error(),
			"type":    "legacy",
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

func (e *legacyMigrationBase) PendingMigrationOperations(ctx context.Context, ns model.Namespace, q map[string]interface{}) int {
	env := e.Env()

	session, err := env.GetSession()
	if err != nil {
		grip.Error(errors.WithStack(err))
		return -1
	}
	defer session.Close()

	coll := session.DB(ns.DB).C(ns.Collection)

	num, err := coll.Find(q).Count()
	if err != nil {
		grip.Warning(errors.WithStack(err))
		return -1
	}

	return num
}

func (e *legacyMigrationBase) GetMigrationEvents(ctx context.Context, q map[string]interface{}) MigrationMetadataIterator {
	env := e.Env()

	session, err := env.GetSession()
	if err != nil {
		return &errorMigrationIterator{err: err}
	}

	ns := env.MetadataNamespace()

	return &legacyMigrationMetadataIterator{
		iter: db.NewCombinedIterator(session, session.DB(ns.DB).C(ns.Collection).Find(q).Iter()),
	}
}
