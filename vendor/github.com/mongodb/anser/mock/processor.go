package mock

import (
	"github.com/mongodb/anser/client"
	"github.com/mongodb/anser/model"
)

type Processor struct {
	NS                      model.Namespace
	Query                   map[string]interface{}
	Cursor                  client.Cursor
	MigrateError            error
	LastMigrateCallMismatch bool
	NumMigrateCalls         int
}

func (p *Processor) Load(cl client.Client, ns model.Namespace, query map[string]interface{}) client.Cursor {
	p.NS = ns
	p.Query = query

	if p.Cursor == nil {
		return nil
	}

	return p.Cursor
}

func (p *Processor) Migrate(cursor client.Cursor) error {
	p.NumMigrateCalls++
	if cursor == p.Cursor {
		p.LastMigrateCallMismatch = false
	} else {
		p.LastMigrateCallMismatch = true
	}

	return p.MigrateError

}
