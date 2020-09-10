package mock

import (
	"context"

	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/scripting"
)

// RemoteManager implements the remote.Manager interface with exported fields
// to configure and introspect the mock's behavior.
type RemoteManager struct {
	Manager
	FailCloseConnection    bool
	FailConfigureCache     bool
	FailDownloadFile       bool
	FailDownloadMongoDB    bool
	FailGetLogStream       bool
	FailGetBuildloggerURLs bool
	FailSignalEvent        bool
	FailCreateScripting    bool
	FailGetScripting       bool
	FailSendMessages       bool

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

	SendMessagePayload options.LoggingPayload
}

// CloseConnection is a no-op. If FailCloseConnection is set, it returns an
// error.
func (c *RemoteManager) CloseConnection() error {
	if c.FailCloseConnection {
		return mockFail()
	}
	return nil
}

// ConfigureCache stores the given cache options. If FailConfigureCache is set,
// it returns an error.
func (c *RemoteManager) ConfigureCache(ctx context.Context, opts options.Cache) error {
	if c.FailConfigureCache {
		return mockFail()
	}

	c.CacheOptions = opts

	return nil
}

// DownloadFile stores the given download options. If FailDownloadFile is set,
// it returns an error.
func (c *RemoteManager) DownloadFile(ctx context.Context, opts options.Download) error {
	if c.FailDownloadFile {
		return mockFail()
	}

	c.DownloadOptions = opts

	return nil
}

// DownloadMongoDB stores the given download options. If FailDownloadMongoDB is
// set, it returns an error.
func (c *RemoteManager) DownloadMongoDB(ctx context.Context, opts options.MongoDBDownload) error {
	if c.FailDownloadMongoDB {
		return mockFail()
	}

	c.MongoDBDownloadOptions = opts

	return nil
}

// GetBuildloggerURLs returns the BuildloggerURLs field. If
// FailGetBuildloggerURLs is set, it returns an error.
func (c *RemoteManager) GetBuildloggerURLs(ctx context.Context, id string) ([]string, error) {
	if c.FailGetBuildloggerURLs {
		return nil, mockFail()
	}

	return c.BuildloggerURLs, nil
}

// GetLogStream stores the given log stream ID and count and returns a
// jasper.LogStream indicating that it is done. If FailGetLogStream is set, it
// returns an error.
func (c *RemoteManager) GetLogStream(ctx context.Context, id string, count int) (jasper.LogStream, error) {
	if c.FailGetLogStream {
		return jasper.LogStream{Done: true}, mockFail()
	}

	c.LogStreamID = id
	c.LogStreamCount = count

	return c.LogStream, nil
}

// SignalEvent stores the given event name. If FailSignalEvent is set, it
// returns an error.
func (c *RemoteManager) SignalEvent(ctx context.Context, name string) error {
	if c.FailSignalEvent {
		return mockFail()
	}

	c.EventName = name

	return nil
}

// SendMessages stores the given logging payload. If FailSendMessages is set, it
// returns an error.
func (c *RemoteManager) SendMessages(ctx context.Context, opts options.LoggingPayload) error {
	if c.FailSendMessages {
		return mockFail()
	}

	c.SendMessagePayload = opts
	return nil
}

// GetScripting returns a cached scripting environment. If FailGetScripting is
// set, it returns an error.
func (c *RemoteManager) GetScripting(ctx context.Context, id string) (scripting.Harness, error) {
	if c.FailGetScripting {
		return nil, mockFail()
	}
	return c.ScriptingEnv, nil
}

// CreateScripting constructs an attached scripting environment. If
// FailCreateScripting is set, it returns an error.
func (c *RemoteManager) CreateScripting(ctx context.Context, opts options.ScriptingHarness) (scripting.Harness, error) {
	if c.FailCreateScripting {
		return nil, mockFail()
	}
	return c.ScriptingEnv, nil
}
