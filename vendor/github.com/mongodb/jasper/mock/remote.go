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
	FailWriteFile          bool

	// ConfigureCache input
	CacheOptions options.Cache

	// DownloadFile input
	DownloadInfo options.Download

	// WriteFile input
	WriteFileInfo options.WriteFile

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

func (c *RemoteClient) CloseConnection() error {
	if c.FailCloseConnection {
		return mockFail()
	}
	return nil
}

func (c *RemoteClient) ConfigureCache(ctx context.Context, opts options.Cache) error {
	if c.FailConfigureCache {
		return mockFail()
	}

	c.CacheOptions = opts

	return nil
}

func (c *RemoteClient) DownloadFile(ctx context.Context, info options.Download) error {
	if c.FailDownloadFile {
		return mockFail()
	}

	c.DownloadInfo = info

	return nil
}

func (c *RemoteClient) DownloadMongoDB(ctx context.Context, opts options.MongoDBDownload) error {
	if c.FailDownloadMongoDB {
		return mockFail()
	}

	c.MongoDBDownloadOptions = opts

	return nil
}

func (c *RemoteClient) GetBuildloggerURLs(ctx context.Context, id string) ([]string, error) {
	if c.FailGetBuildloggerURLs {
		return nil, mockFail()
	}

	return c.BuildloggerURLs, nil
}

func (c *RemoteClient) GetLogStream(ctx context.Context, id string, count int) (jasper.LogStream, error) {
	if c.FailGetLogStream {
		return jasper.LogStream{Done: true}, mockFail()
	}
	c.LogStreamID = id
	c.LogStreamCount = count

	return jasper.LogStream{Done: true}, nil
}

func (c *RemoteClient) SignalEvent(ctx context.Context, name string) error {
	if c.FailSignalEvent {
		return mockFail()
	}

	c.EventName = name

	return nil
}

func (c *RemoteClient) WriteFile(ctx context.Context, info options.WriteFile) error {
	if c.FailWriteFile {
		return mockFail()
	}

	c.WriteFileInfo = info

	return nil
}
