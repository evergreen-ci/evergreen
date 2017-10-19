package testutil

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/amboy"
	"github.com/mongodb/anser/db"
)

var _ evergreen.Environment = &Environment{}

type Environment struct {
	Remote            amboy.Queue
	Local             amboy.Queue
	DBSession         db.Session
	EvergreenSettings *evergreen.Settings
}

func (e *Environment) Configure(ctx context.Context, path string) error {
	e.EvergreenSettings = TestConfig()
	return nil
}

func (e *Environment) RemoteQueue() amboy.Queue      { return e.Remote }
func (e *Environment) LocalQueue() amboy.Queue       { return e.Local }
func (e *Environment) Session() db.Session           { return e.DBSession }
func (e *Environment) Settings() *evergreen.Settings { return e.EvergreenSettings }
