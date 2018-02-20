package rest

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/tychoish/gimlet"
)

const (
	defaultClientPort int = 3000
	maxClientPort         = 65535
)

// Client provides an interface for interacting with a remote amboy
// Service.
type Client struct {
	host   string
	prefix string
	port   int
	client *http.Client
}

// NewClient takes host, port, and URI prefix information and
// constructs a new Client.
func NewClient(host string, port int, prefix string) (*Client, error) {
	c := &Client{client: &http.Client{}}

	return c.initClient(host, port, prefix)
}

// NewClientFromExisting takes an existing http.Client object and
// produces a new Client object.
func NewClientFromExisting(client *http.Client, host string, port int, prefix string) (*Client, error) {
	if client == nil {
		return nil, errors.New("must use a non-nil existing client")
	}

	c := &Client{client: client}

	return c.initClient(host, port, prefix)
}

// Copy takes an existing Client object and returns a new client
// object with the same settings that uses a *new* http.Client.
func (c *Client) Copy() *Client {
	new := &Client{}
	*new = *c
	new.client = &http.Client{}

	return new
}

func (c *Client) initClient(host string, port int, prefix string) (*Client, error) {
	err := c.SetHost(host)
	if err != nil {
		return nil, err
	}

	err = c.SetPort(port)
	if err != nil {
		return nil, err
	}

	err = c.SetPrefix(prefix)
	if err != nil {
		return nil, err
	}

	return c, nil
}

////////////////////////////////////////////////////////////////////////
//
// Configuration Interface
//
////////////////////////////////////////////////////////////////////////

// Client returns a pointer to embedded http.Client object.
func (c *Client) Client() *http.Client {
	return c.client
}

// SetHost allows callers to change the hostname (including leading
// "http(s)") for the Client. Returns an error if the specified host
// does not start with "http".
func (c *Client) SetHost(h string) error {
	if !strings.HasPrefix(h, "http") {
		return errors.Errorf("host '%s' is malformed. must start with 'http'", h)
	}

	if strings.HasSuffix(h, "/") {
		h = h[:len(h)-1]
	}

	c.host = h

	return nil
}

// Host returns the current host.
func (c *Client) Host() string {
	return c.host
}

// SetPort allows callers to change the port used for the client. If
// the port is invalid, returns an error and sets the port to the
// default value. (3000)
func (c *Client) SetPort(p int) error {
	if p <= 0 || p >= maxClientPort {
		c.port = defaultClientPort
		return errors.Errorf("cannot set the port to %d, using %d instead", p, defaultClientPort)
	}

	c.port = p
	return nil
}

// Port returns the current port value for the Client.
func (c *Client) Port() int {
	return c.port
}

// SetPrefix allows callers to modify the prefix, for this client,
func (c *Client) SetPrefix(p string) error {
	c.prefix = strings.Trim(p, "/")
	return nil
}

// Prefix accesses the prefix for the client, The prefix is the part
// of the URI between the end-point and the hostname, of the API.
func (c *Client) Prefix() string {
	return c.prefix
}

func (c *Client) getURL(endpoint string) string {
	var url []string

	if c.port == 80 || c.port == 0 {
		url = append(url, c.host)
	} else {
		url = append(url, fmt.Sprintf("%s:%d", c.host, c.port))
	}

	if c.prefix != "" {
		url = append(url, c.prefix)
	}

	if endpoint = strings.Trim(endpoint, "/"); endpoint != "" {
		url = append(url, endpoint)
	}

	return strings.Join(url, "/")
}

////////////////////////////////////////////////////////////////////////
//
// Operations that Interact with the Remote API.
//
////////////////////////////////////////////////////////////////////////

func (c *Client) getStats(ctx context.Context) (*status, error) {

	req, err := http.NewRequest(http.MethodGet, c.getURL("/v1/status"), nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	s := &status{}
	if err = gimlet.GetJSON(resp.Body, s); err != nil {
		return nil, err
	}

	return s, nil
}

// Running is true when the underlying queue is running and accepting
// jobs, and false when the queue is not runner or if there's a
// problem connecting to the queue.
func (c *Client) Running(ctx context.Context) (bool, error) {
	s, err := c.getStats(ctx)
	if err != nil {
		return false, err
	}

	return s.QueueRunning, nil
}

// PendingJobs reports on the total number of jobs currently dispatched
// by the queue to workers.
func (c *Client) PendingJobs(ctx context.Context) (int, error) {
	s, err := c.getStats(ctx)
	if err != nil {
		return -1, err
	}

	return s.PendingJobs, nil
}

// SubmitJob adds a job to a remote queue connected to the rest interface.
func (c *Client) SubmitJob(ctx context.Context, j amboy.Job) (string, error) {
	ji, err := registry.MakeJobInterchange(j, amboy.JSON)
	if err != nil {
		return "", err
	}

	b, err := amboy.ConvertTo(amboy.JSON, ji)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, c.getURL("/v1/job/create"), bytes.NewBuffer(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(ctx)

	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	cjr := createResponse{}

	if err = gimlet.GetJSON(resp.Body, &cjr); err != nil {
		return "", err
	}

	if cjr.Error != "" {
		return "", errors.Errorf("service reported error: '%s'", cjr.Error)
	}

	return cjr.ID, nil
}

// FetchJob takes the name of a queue, and returns if possible a
// representation of that job object.
func (c *Client) FetchJob(ctx context.Context, name string) (amboy.Job, error) {
	req, err := http.NewRequest(http.MethodGet, c.getURL(fmt.Sprintf("/v1/job/%s", name)), nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	ji := &registry.JobInterchange{}
	if err = gimlet.GetJSON(resp.Body, ji); err != nil {
		return nil, err
	}

	j, err := registry.ConvertToJob(ji, amboy.JSON)
	if err != nil {
		return nil, err
	}

	return j, nil
}

func (c *Client) jobStatus(ctx context.Context, name string) (*jobStatusResponse, error) {
	req, err := http.NewRequest(http.MethodGet, c.getURL(fmt.Sprintf("/v1/job/status/%s", name)), nil)
	if err != nil {
		return nil, err
	}
	req = req.WithContext(ctx)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	s := &jobStatusResponse{}
	if err = gimlet.GetJSON(resp.Body, s); err != nil {
		return nil, err
	}

	return s, nil
}

// JobComplete checks the stats of a job, by name, and returns true if
// that job is complete. When false, check the second return value to
// ensure that the job exists in the remote queue.
func (c *Client) JobComplete(ctx context.Context, name string) (bool, error) {
	st, err := c.jobStatus(ctx, name)
	if err != nil {
		return false, err
	}

	return st.Completed, nil
}

// Wait blocks until the job identified by the name argument is
// complete. Does not handle the case where a job does not exist.
func (c *Client) Wait(ctx context.Context, name string) bool {
	timeout := 20 * time.Second
	deadline, ok := ctx.Deadline()
	if ok {
		timeout = time.Since(deadline)
	}

	req, err := http.NewRequest(http.MethodGet, c.getURL(fmt.Sprintf("/v1/job/wait/%s?timeout=%s", name, timeout)), nil)
	if err != nil {
		return false
	}
	req = req.WithContext(ctx)

	resp, err := c.client.Do(req)
	if err != nil {
		grip.Info(err)
		grip.Debugf("%+v", resp)
		return false
	}
	return true
}

// WaitAll waits for *all* pending jobs in the queue to complete.
func (c *Client) WaitAll(ctx context.Context) bool {
	timeout := 20 * time.Second
	deadline, ok := ctx.Deadline()
	if ok {
		timeout = time.Since(deadline)
	}

	req, err := http.NewRequest(http.MethodGet, c.getURL(fmt.Sprintf("/v1/status/wait?timeout=%s", timeout)), nil)
	if err != nil {
		return false
	}
	req = req.WithContext(ctx)

	resp, err := c.client.Do(req)
	if err != nil {
		grip.Info(err)
		grip.Debugf("%+v", resp)
		return false
	}

	return true
}
