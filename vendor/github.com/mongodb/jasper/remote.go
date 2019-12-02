package jasper

import (
	"context"

	"github.com/mongodb/jasper/options"
)

// CloseFunc is a function used to close a service or close the client
// connection to a service.
type CloseFunc func() error

// RemoteClient provides an interface to access all functionality from a Jasper
// service. It includes an interface to interact with Jasper Managers and
// Processes remotely as well as access to remote-specific functionality.
type RemoteClient interface {
	Manager
	CloseConnection() error
	ConfigureCache(ctx context.Context, opts options.Cache) error
	DownloadFile(ctx context.Context, opts options.Download) error
	DownloadMongoDB(ctx context.Context, opts options.MongoDBDownload) error
	GetLogStream(ctx context.Context, id string, count int) (LogStream, error)
	GetBuildloggerURLs(ctx context.Context, id string) ([]string, error)
	SignalEvent(ctx context.Context, name string) error
}
