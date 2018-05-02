package mock

import (
	"context"
	"runtime"
	"sync"

	"github.com/evergreen-ci/evergreen"
	edb "github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/anser/db"
	anserMock "github.com/mongodb/anser/mock"
	"github.com/mongodb/grip/send"
)

// this is just a hack to ensure that compile breaks clearly if the
// mock implementation diverges from the interface
var _ evergreen.Environment = &Environment{}

type Environment struct {
	Remote            amboy.Queue
	Driver            queue.Driver
	Local             amboy.Queue
	DBSession         *anserMock.Session
	EvergreenSettings *evergreen.Settings
	mu                sync.RWMutex

	InternalSender *send.InternalSender
}

func (e *Environment) Configure(ctx context.Context, path string, db *evergreen.DBSettings) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.EvergreenSettings = testutil.TestConfig()
	if db != nil {
		e.EvergreenSettings.Database = *db
	}
	e.DBSession = anserMock.NewSession()
	e.Driver = queue.NewPriorityDriver()

	if err := e.Driver.Open(ctx); err != nil {
		return err
	}

	rq := queue.NewRemoteUnordered(2)
	if err := rq.SetDriver(e.Driver); err != nil {
		return err
	}
	e.Remote = rq
	e.Local = queue.NewLocalUnordered(2)

	edb.SetGlobalSessionProvider(e.EvergreenSettings.SessionFactory())

	e.InternalSender = send.MakeInternalLogger()

	return nil
}

func (e *Environment) RemoteQueue() amboy.Queue {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.Remote
}

func (e *Environment) LocalQueue() amboy.Queue {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.Local
}

func (e *Environment) Session() db.Session {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.DBSession
}

func (e *Environment) Settings() *evergreen.Settings {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.EvergreenSettings
}

func (e *Environment) ClientConfig() *evergreen.ClientConfig {
	return &evergreen.ClientConfig{
		LatestRevision: evergreen.ClientVersion,
		ClientBinaries: []evergreen.ClientBinary{
			evergreen.ClientBinary{
				URL:  "https://example.com/clients/evergreen",
				OS:   runtime.GOOS,
				Arch: runtime.GOARCH,
			},
		},
	}
}

func (e *Environment) GetSender(key evergreen.SenderKey) (send.Sender, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.InternalSender, nil
}
