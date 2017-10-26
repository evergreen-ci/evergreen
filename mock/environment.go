package mock

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	edb "github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/amboy/queue/driver"
	"github.com/mongodb/anser/db"
	anserMock "github.com/mongodb/anser/mock"
)

// this is just a hack to ensure that compile breaks clearly if the
// mock implementation diverges from the interface
var _ evergreen.Environment = &Environment{}

type Environment struct {
	Remote            amboy.Queue
	Driver            *driver.Priority
	Local             amboy.Queue
	DBSession         *anserMock.Session
	EvergreenSettings *evergreen.Settings
}

func (e *Environment) Configure(ctx context.Context, path string) error {
	e.EvergreenSettings = testutil.TestConfig()
	e.DBSession = anserMock.NewSession()
	e.Driver = driver.NewPriority()

	if err := e.Driver.Open(ctx); err != nil {
		return err
	}

	rq := queue.NewRemoteUnordered(2)
	rq.SetDriver(e.Driver)
	e.Remote = rq
	e.Local = queue.NewLocalUnordered(2)

	edb.SetGlobalSessionProvider(e.EvergreenSettings.SessionFactory())

	return nil
}

func (e *Environment) RemoteQueue() amboy.Queue      { return e.Remote }
func (e *Environment) LocalQueue() amboy.Queue       { return e.Local }
func (e *Environment) Session() db.Session           { return e.DBSession }
func (e *Environment) Settings() *evergreen.Settings { return e.EvergreenSettings }
