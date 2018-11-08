package anser

import (
	"sync"

	"github.com/mongodb/amboy/job"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

// MigrationHelper is an interface embedded in all jobs as an
// "extended base" for migrations on top of the existing amboy.Base
// type which implements most job functionality.
//
// MigrationHelper implementations should not require construction:
// getter methods should initialize nil values at runtime.
type MigrationHelper interface {
	Env() Environment

	// Migrations need to record their state to help resolve
	// dependencies to the database.
	FinishMigration(string, *job.Base)
	SaveMigrationEvent(*model.MigrationMetadata) error

	// The migration helper provides a model/interface for
	// interacting with the database to check the state of a
	// migration operation, helpful in dependency approval.
	PendingMigrationOperations(model.Namespace, map[string]interface{}) int
	GetMigrationEvents(map[string]interface{}) (db.Iterator, error)
}

// NewMigrationHelper constructs a new migration helper instance. Use
// this to inject environment instances into tasks.
func NewMigrationHelper(e Environment) MigrationHelper { return &migrationBase{env: e} }

type migrationBase struct {
	env Environment
	sync.Mutex
}

func (e *migrationBase) Env() Environment {
	e.Lock()
	defer e.Unlock()

	if e.env == nil {
		e.env = GetEnvironment()
	}

	return e.env
}

func (e *migrationBase) SaveMigrationEvent(m *model.MigrationMetadata) error {
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

func (e *migrationBase) FinishMigration(name string, j *job.Base) {
	j.MarkComplete()
	meta := model.MigrationMetadata{
		ID:        j.ID(),
		Migration: name,
		HasErrors: j.HasErrors(),
		Completed: true,
	}
	err := e.SaveMigrationEvent(&meta)
	if err != nil {
		j.AddError(err)
		grip.Warning(message.Fields{
			"message": "encountered problem saving migration event",
			"id":      j.ID(),
			"name":    name,
			"error":   err.Error(),
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

func (e *migrationBase) PendingMigrationOperations(ns model.Namespace, q map[string]interface{}) int {
	env := e.Env()

	session, err := env.GetSession()
	if err != nil {
		grip.Error(errors.WithStack(err))
		return -1
	}
	defer session.Close()

	coll := session.DB(ns.DB).C(ns.Collection)

	num, err := coll.Find(bson.M(q)).Count()
	if err != nil {
		grip.Warning(errors.WithStack(err))
		return -1
	}

	return num
}

func (e *migrationBase) GetMigrationEvents(q map[string]interface{}) (db.Iterator, error) {
	env := e.Env()
	ns := env.MetadataNamespace()

	session, err := env.GetSession()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	iter := db.NewCombinedIterator(session, session.DB(ns.DB).C(ns.Collection).Find(bson.M(q)).Iter())

	return iter, nil
}
