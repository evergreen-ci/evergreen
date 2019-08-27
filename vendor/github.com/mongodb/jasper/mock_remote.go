package jasper

import (
	"context"
)

// MockRemoteClient implements the RemoteClient interface with exported fields
// to configure and introspect the mock's behavior.
type MockRemoteClient struct {
	MockManager
	FailCloseConnection    bool
	FailConfigureCache     bool
	FailDownloadFile       bool
	FailDownloadMongoDB    bool
	FailGetLogStream       bool
	FailGetBuildloggerURLs bool
	FailSignalEvent        bool
	FailWriteFile          bool

	// ConfigureCache input
	CacheOptions CacheOptions

	// DownloadFile input
	DownloadInfo DownloadInfo

	// WriteFile input
	WriteFileInfo WriteFileInfo

	// DownloadMongoDB input
	MongoDBDownloadOptions MongoDBDownloadOptions

	// LogStream input/output
	LogStreamID    string
	LogStreamCount int
	LogStream

	// GetBuildloggerURLs output
	BuildloggerURLs []string

	EventName string
}

func (c *MockRemoteClient) CloseConnection() error {
	if c.FailCloseConnection {
		return mockFail()
	}
	return nil
}

func (c *MockRemoteClient) ConfigureCache(ctx context.Context, opts CacheOptions) error {
	if c.FailConfigureCache {
		return mockFail()
	}

	c.CacheOptions = opts

	return nil
}

func (c *MockRemoteClient) DownloadFile(ctx context.Context, info DownloadInfo) error {
	if c.FailDownloadFile {
		return mockFail()
	}

	c.DownloadInfo = info

	return nil
}

func (c *MockRemoteClient) DownloadMongoDB(ctx context.Context, opts MongoDBDownloadOptions) error {
	if c.FailDownloadMongoDB {
		return mockFail()
	}

	c.MongoDBDownloadOptions = opts

	return nil
}

func (c *MockRemoteClient) GetBuildloggerURLs(ctx context.Context, id string) ([]string, error) {
	if c.FailGetBuildloggerURLs {
		return nil, mockFail()
	}

	return c.BuildloggerURLs, nil
}

func (c *MockRemoteClient) GetLogStream(ctx context.Context, id string, count int) (LogStream, error) {
	if c.FailGetLogStream {
		return LogStream{Done: true}, mockFail()
	}
	c.LogStreamID = id
	c.LogStreamCount = count

	return LogStream{Done: true}, nil
}

func (c *MockRemoteClient) SignalEvent(ctx context.Context, name string) error {
	if c.FailSignalEvent {
		return mockFail()
	}

	c.EventName = name

	return nil
}

func (c *MockRemoteClient) WriteFile(ctx context.Context, info WriteFileInfo) error {
	if c.FailWriteFile {
		return mockFail()
	}

	c.WriteFileInfo = info

	return nil
}
