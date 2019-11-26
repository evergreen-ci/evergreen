package mock

import (
	"context"

	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
)

// RemoteClient implements the RemoteClient interface with exported fields
// to configure and introspect the mock's behavior.
type RemoteClient struct {
	Manager
	FailCloseConnection    bool
	FailConfigureCache     bool
	FailDownloadFile       bool
	FailDownloadMongoDB    bool
	FailGetLogStream       bool
	FailGetBuildloggerURLs bool
	FailSignalEvent        bool

	// ConfigureCache input
	CacheOptions options.Cache

	// DownloadFile input
	DownloadOptions options.Download

	// DownloadMongoDB input
	MongoDBDownloadOptions options.MongoDBDownload

	// LogStream input/output
	LogStreamID    string
	LogStreamCount int
	jasper.LogStream

	// GetBuildloggerURLs output
	BuildloggerURLs []string

	EventName string
}

// CloseConnection is a no-op. If FailCloseConnection is set, it returns an
// error.
func (c *RemoteClient) CloseConnection() error {
	if c.FailCloseConnection {
		return mockFail()
	}
	return nil
}

// ConfigureCache saves the mock CacheOptions. If FailConfigureCache is set, it
// returns an error.
func (c *RemoteClient) ConfigureCache(ctx context.Context, opts options.Cache) error {
	if c.FailConfigureCache {
		return mockFail()
	}

	c.CacheOptions = opts

	return nil
}

// DownloadFile saves the mock DownloadOptions. If FailDownloadFile is set, it
// returns an error.
func (c *RemoteClient) DownloadFile(ctx context.Context, opts options.Download) error {
	if c.FailDownloadFile {
		return mockFail()
	}

	c.DownloadOptions = opts

	return nil
}

// DownloadMongoDB saves the MongoDBDownloadOptions. If FailDownloadMongoDB is
// set, it returns an error.
func (c *RemoteClient) DownloadMongoDB(ctx context.Context, opts options.MongoDBDownload) error {
	if c.FailDownloadMongoDB {
		return mockFail()
	}

	c.MongoDBDownloadOptions = opts

	return nil
}

// GetBuildloggerURLs returns the URLs set in BuildloggerURLs. If
// FailGetBuildloggerURLs is set, it returns an error.
func (c *RemoteClient) GetBuildloggerURLs(ctx context.Context, id string) ([]string, error) {
	if c.FailGetBuildloggerURLs {
		return nil, mockFail()
	}

	return c.BuildloggerURLs, nil
}

// GetLogStream saves the LogStreamID and LogStreamCount and returns a
// jasper.LogStream that is done. If FailGetLogStream is set, it returns an
// error.
func (c *RemoteClient) GetLogStream(ctx context.Context, id string, count int) (jasper.LogStream, error) {
	if c.FailGetLogStream {
		return jasper.LogStream{Done: true}, mockFail()
	}
	c.LogStreamID = id
	c.LogStreamCount = count

	return jasper.LogStream{Done: true}, nil
}

// SignalEvent saves the EventName. If FailSignalEvent is set, it returns an
// error.
func (c *RemoteClient) SignalEvent(ctx context.Context, name string) error {
	if c.FailSignalEvent {
		return mockFail()
	}

	c.EventName = name

	return nil
}
