package mock

import (
	"context"
	"runtime"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	edb "github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	anserMock "github.com/mongodb/anser/mock"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	mgo "gopkg.in/mgo.v2"
)

// this is just a hack to ensure that compile breaks clearly if the
// mock implementation diverges from the interface
var _ evergreen.Environment = &Environment{}

type Environment struct {
	Remote               amboy.Queue
	Driver               queue.Driver
	Local                amboy.Queue
	JasperProcessManager jasper.Manager
	SingleWorker         amboy.Queue
	Closers              map[string]func(context.Context) error
	DBSession            *anserMock.Session
	EvergreenSettings    *evergreen.Settings
	mu                   sync.RWMutex

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

	jpm, err := jasper.NewLocalManager(true)
	if err != nil {
		return errors.WithStack(err)
	}

	e.JasperProcessManager = jpm

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

func (e *Environment) GenerateTasksQueue() amboy.Queue {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.SingleWorker
}

func (e *Environment) Session() *mgo.Session {
	session, _, _ := edb.GetGlobalSessionFactory().GetSession()

	return session
}

func (e *Environment) JasperManager() jasper.Manager {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.JasperProcessManager
}

func (e *Environment) Settings() *evergreen.Settings {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.EvergreenSettings
}

func (e *Environment) SaveConfig() error {
	return nil
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

func (e *Environment) RegisterCloser(name string, closer func(context.Context) error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.Closers[name] = closer
}

func (e *Environment) Close(ctx context.Context) error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// TODO we could, in the future call all closers in but that
	// would require more complex waiting and timeout logic

	deadline, _ := ctx.Deadline()
	catcher := grip.NewBasicCatcher()
	for name, closer := range e.Closers {
		if closer == nil {
			continue
		}

		grip.Info(message.Fields{
			"message":      "calling closer",
			"closer":       name,
			"timeout_secs": time.Since(deadline),
			"deadline":     deadline,
		})
		catcher.Add(closer(ctx))
	}

	return catcher.Resolve()
}
