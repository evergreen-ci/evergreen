package testresults

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/juniper/gopb"
	"github.com/evergreen-ci/timber"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

// Client provides a wrapper around a gRPC client for sending test results to Cedar.
type Client struct {
	client    gopb.CedarTestResultsClient
	closeConn func() error
	closed    bool
}

// NewClient returns a Client to send test results to Cedar. If authentication credentials are not
// specified, then an insecure connection will be established with the specified address and port.
func NewClient(ctx context.Context, opts timber.ConnectionOptions) (*Client, error) {
	var conn *grpc.ClientConn
	var err error

	if err = opts.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid connection options")
	}

	if opts.DialOpts.APIKey == "" {
		addr := fmt.Sprintf("%s:%s", opts.DialOpts.BaseAddress, opts.DialOpts.RPCPort)
		conn, err = grpc.DialContext(ctx, addr, grpc.WithInsecure())
	} else {
		conn, err = timber.DialCedar(ctx, &opts.Client, opts.DialOpts)
	}
	if err != nil {
		return nil, errors.Wrap(err, "problem dialing rpc server")
	}

	s := &Client{
		client:    gopb.NewCedarTestResultsClient(conn),
		closeConn: conn.Close,
	}
	return s, nil
}

// NewClientWithExistingConnection returns a Client to send test results to Cedar using the
// given client connection. The given client connection's lifetime will not be managed by this
// client.
func NewClientWithExistingConnection(ctx context.Context, conn *grpc.ClientConn) (*Client, error) {
	if conn == nil {
		return nil, errors.New("must provide an existing client connection")
	}

	s := &Client{
		client:    gopb.NewCedarTestResultsClient(conn),
		closeConn: func() error { return nil },
	}
	return s, nil
}

// CreateRecord creates a new metadata record in Cedar with the given options.
func (c *Client) CreateRecord(ctx context.Context, opts CreateOptions) (string, error) {
	resp, err := c.client.CreateTestResultsRecord(ctx, opts.export())
	if err != nil {
		return "", errors.WithStack(err)
	}
	return resp.TestResultsRecordId, nil
}

// AddResults adds a set of test results to the record.
func (c *Client) AddResults(ctx context.Context, r Results) error {
	if err := r.validate(); err != nil {
		return errors.Wrap(err, "invalid test results")
	}

	exported, err := r.export()
	if err != nil {
		return errors.Wrap(err, "converting test results to protobuf type")
	}

	if _, err := c.client.AddTestResults(ctx, exported); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// CloseRecord marks a record as completed.
func (c *Client) CloseRecord(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("id cannot be empty")
	}

	if _, err := c.client.CloseTestResultsRecord(ctx, &gopb.TestResultsEndInfo{TestResultsRecordId: id}); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

// CloseClient closes the client connection if it was created via NewClient. If an existing
// connection was used to create the client, it will not be closed.
func (c *Client) CloseClient() error {
	if c.closed {
		return nil
	}
	if c.closeConn == nil {
		return nil
	}
	return c.closeConn()
}
