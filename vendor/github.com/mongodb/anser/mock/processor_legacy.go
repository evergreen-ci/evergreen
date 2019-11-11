package mock

import (
	"github.com/mongodb/anser/db"
	"github.com/mongodb/anser/model"
)

type LegacyProcessor struct {
	NS                      model.Namespace
	Query                   map[string]interface{}
	Iter                    *Iterator
	MigrateError            error
	LastMigrateCallMismatch bool
	NumMigrateCalls         int
}

func (p *LegacyProcessor) Load(session db.Session, ns model.Namespace, query map[string]interface{}) db.Iterator {
	p.NS = ns
	p.Query = query

	if p.Iter == nil {
		return nil
	}

	return p.Iter
}

func (p *LegacyProcessor) Migrate(iter db.Iterator) error {
	p.NumMigrateCalls++
	if iter == p.Iter {
		p.LastMigrateCallMismatch = false
	} else {
		p.LastMigrateCallMismatch = true
	}

	return p.MigrateError

}
