package jasper

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
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
	ConfigureCache(context.Context, CacheOptions) error
	DownloadFile(context.Context, DownloadInfo) error
	DownloadMongoDB(context.Context, MongoDBDownloadOptions) error
	GetLogStream(context.Context, string, int) (LogStream, error)
	GetBuildloggerURLs(context.Context, string) ([]string, error)
	SignalEvent(context.Context, string) error
}

// RemoteOptions represents options to SSH into a remote machine.
type RemoteOptions struct {
	Host string
	User string
	Args []string
}

// Validate checks that the host is set so that the remote host can be
// identified.
func (opts *RemoteOptions) Validate() error {
	if opts.Host == "" {
		return errors.New("host cannot be empty")
	}
	return nil
}

func (opts *RemoteOptions) hostString() string {
	if opts.User == "" {
		return opts.Host
	}

	return fmt.Sprintf("%s@%s", opts.User, opts.Host)
}
