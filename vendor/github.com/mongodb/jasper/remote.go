package jasper

import "context"

// CloseFunc is a function used to close a service or close the client
// connection to a service.
type CloseFunc func() error

// RemoteClient provides an interface to access all functionality from the
// Jasper REST service. It includes an interface to interact with Jasper
// Managers remotely as well as access to remote-specific functionality.
type RemoteClient interface {
	Manager
	CloseConnection() error
	ConfigureCache(context.Context, CacheOptions) error
	DownloadFile(context.Context, DownloadInfo) error
	DownloadMongoDB(context.Context, MongoDBDownloadOptions) error
	GetBuildloggerURLs(ctx context.Context, name string) ([]string, error)
	SignalEvent(ctx context.Context, name string) error
}
