package remote

import (
	"context"

	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/scripting"
)

// Manager provides an interface to access all functionality from a Jasper
// service. It includes an interface to interact with Jasper Managers and
// Processes remotely as well as access to remote-specific functionality.
type Manager interface {
	jasper.Manager

	CloseConnection() error
	ConfigureCache(ctx context.Context, opts options.Cache) error
	DownloadFile(ctx context.Context, opts options.Download) error
	DownloadMongoDB(ctx context.Context, opts options.MongoDBDownload) error
	GetLogStream(ctx context.Context, id string, count int) (jasper.LogStream, error)
	GetBuildloggerURLs(ctx context.Context, id string) ([]string, error)
	SignalEvent(ctx context.Context, name string) error

	CreateScripting(context.Context, options.ScriptingHarness) (scripting.Harness, error)
	GetScripting(context.Context, string) (scripting.Harness, error)

	SendMessages(context.Context, options.LoggingPayload) error
}
