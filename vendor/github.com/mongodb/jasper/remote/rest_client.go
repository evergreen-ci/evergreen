package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"

	"github.com/evergreen-ci/bond"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/scripting"
	"github.com/pkg/errors"
)

// NewRESTClient creates a REST client that connects to the given address
// running the Jasper REST service.
func NewRESTClient(addr net.Addr) Manager {
	return &restClient{
		prefix: fmt.Sprintf("http://%s/jasper/v1", addr),
		client: bond.GetHTTPClient(),
	}
}

type restClient struct {
	prefix string
	client *http.Client
}

func (c *restClient) CloseConnection() error {
	bond.PutHTTPClient(c.client)
	return nil
}

func (c *restClient) getURL(route string, args ...interface{}) string {
	if !strings.HasPrefix(route, "/") {
		route = "/" + route
	}

	if len(args) == 0 {
		return c.prefix + route
	}

	return fmt.Sprintf(c.prefix+route, args...)
}

func makeBody(data interface{}) (io.Reader, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, errors.Wrap(err, "problem marshaling request body")
	}

	return bytes.NewBuffer(payload), nil
}

func handleError(resp *http.Response) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}

	wrapError := func(err error) error {
		return errors.Wrapf(err, "HTTP status code %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return wrapError(errors.Wrap(err, "reading response body"))
	}
	gimerr := gimlet.ErrorResponse{}
	if err := json.Unmarshal(body, &gimerr); err != nil {
		return wrapError(errors.Errorf("received response: %s", string(body)))
	}

	return wrapError(gimerr)
}

func (c *restClient) doRequest(ctx context.Context, method string, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, errors.Wrap(err, "problem building request")
	}

	req = req.WithContext(ctx)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "problem making request")
	}

	if err = handleError(resp); err != nil {
		return nil, errors.WithStack(err)
	}

	return resp, nil
}

func (c *restClient) ID() string {
	resp, err := c.doRequest(context.Background(), http.MethodGet, c.getURL("/id"), nil)
	if err != nil {
		grip.Debug(errors.Wrap(err, "request returned error"))
		return ""
	}
	defer resp.Body.Close()

	var id string
	if err = gimlet.GetJSON(resp.Body, &id); err != nil {
		return ""
	}

	return id
}

func (c *restClient) CreateProcess(ctx context.Context, opts *options.Create) (jasper.Process, error) {
	body, err := makeBody(opts)
	if err != nil {
		return nil, errors.Wrap(err, "problem building request for job create")
	}

	resp, err := c.doRequest(ctx, http.MethodPost, c.getURL("/create"), body)
	if err != nil {
		return nil, errors.Wrap(err, "request returned error")
	}
	defer resp.Body.Close()

	var info jasper.ProcessInfo
	if err := gimlet.GetJSON(resp.Body, &info); err != nil {
		return nil, errors.Wrap(err, "problem reading process info from response")
	}

	return &restProcess{
		id:     info.ID,
		client: c,
	}, nil
}

func (c *restClient) CreateCommand(ctx context.Context) *jasper.Command {
	return jasper.NewCommand().ProcConstructor(c.CreateProcess)
}

func (c *restClient) CreateScripting(ctx context.Context, opts options.ScriptingHarness) (scripting.Harness, error) {
	if err := opts.Validate(); err != nil {
		return nil, errors.Wrap(err, "problem validating input")
	}

	body, err := makeBody(opts)
	if err != nil {
		return nil, errors.Wrap(err, "problem building request for scripting create")
	}

	resp, err := c.doRequest(ctx, http.MethodPost, c.getURL("/scripting/create/%s", opts.Type()), body)
	if err != nil {
		return nil, errors.Wrap(err, "request returned error")
	}
	defer resp.Body.Close()

	if err = handleError(resp); err != nil {
		return nil, errors.WithStack(err)
	}

	out := struct {
		ID string `json:"id"`
	}{}

	if err = gimlet.GetJSON(resp.Body, &out); err != nil {
		return nil, errors.Wrap(err, "problem reading response")
	}

	return newRESTScriptingHarness(c, out.ID), nil
}

func (c *restClient) GetScripting(ctx context.Context, id string) (scripting.Harness, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, c.getURL("/scripting/%s", id), nil)
	if err != nil {
		return nil, errors.Wrap(err, "request returned error")
	}
	defer resp.Body.Close()

	if err = handleError(resp); err != nil {
		return nil, errors.WithStack(err)
	}

	return newRESTScriptingHarness(c, id), nil
}

func (c *restClient) Register(ctx context.Context, proc jasper.Process) error {
	return errors.New("cannot register a local process on a remote service")
}

func (c *restClient) getListOfProcesses(resp *http.Response) ([]jasper.Process, error) {
	payload := []jasper.ProcessInfo{}
	if err := gimlet.GetJSON(resp.Body, &payload); err != nil {
		return nil, errors.Wrap(err, "problem reading process info from response")
	}

	output := []jasper.Process{}
	for _, info := range payload {
		output = append(output, &restProcess{
			id:     info.ID,
			client: c,
		})
	}

	return output, nil
}

func (c *restClient) List(ctx context.Context, f options.Filter) ([]jasper.Process, error) {
	if err := f.Validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	resp, err := c.doRequest(ctx, http.MethodGet, c.getURL("/list/%s", string(f)), nil)
	if err != nil {
		return nil, errors.Wrap(err, "request returned error")
	}
	defer resp.Body.Close()

	if err = handleError(resp); err != nil {
		return nil, errors.WithStack(err)
	}

	out, err := c.getListOfProcesses(resp)

	return out, errors.WithStack(err)
}

func (c *restClient) Group(ctx context.Context, name string) ([]jasper.Process, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, c.getURL("/list/group/%s", name), nil)
	if err != nil {
		return nil, errors.Wrap(err, "request returned error")
	}
	defer resp.Body.Close()

	if err = handleError(resp); err != nil {
		return nil, errors.WithStack(err)
	}

	out, err := c.getListOfProcesses(resp)

	return out, errors.WithStack(err)
}

func (c *restClient) getProcess(ctx context.Context, id string) (*http.Response, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, c.getURL("/process/%s", id), nil)
	if err != nil {
		return nil, errors.Wrap(err, "request returned error")
	}

	return resp, nil
}

func (c *restClient) getProcessInfo(ctx context.Context, id string) (jasper.ProcessInfo, error) {
	resp, err := c.getProcess(ctx, id)
	if err != nil {
		return jasper.ProcessInfo{}, errors.WithStack(err)
	}
	defer resp.Body.Close()

	out := jasper.ProcessInfo{}
	if err = gimlet.GetJSON(resp.Body, &out); err != nil {
		return jasper.ProcessInfo{}, errors.WithStack(err)
	}

	return out, nil
}

func (c *restClient) Get(ctx context.Context, id string) (jasper.Process, error) {
	resp, err := c.getProcess(ctx, id)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer resp.Body.Close()

	// we don't actually need to parse the body of the post if we
	// know the process exists.
	return &restProcess{
		id:     id,
		client: c,
	}, nil
}

func (c *restClient) Clear(ctx context.Context) {
	// Avoid errors here, because we can't return them anyways, and these errors
	// should not really ever happen.
	resp, err := c.doRequest(ctx, http.MethodPost, c.getURL("/clear"), nil)
	if err != nil {
		grip.Debug(errors.Wrap(err, "request returned error"))
	}
	defer resp.Body.Close()
}

func (c *restClient) Close(ctx context.Context) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, c.getURL("/close"), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (c *restClient) GetBuildloggerURLs(ctx context.Context, id string) ([]string, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, c.getURL("/process/%s/buildlogger-urls", id), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	urls := []string{}
	if err = gimlet.GetJSON(resp.Body, &urls); err != nil {
		return nil, errors.Wrap(err, "problem reading urls from response")
	}

	return urls, nil
}

func (c *restClient) GetLogStream(ctx context.Context, id string, count int) (jasper.LogStream, error) {
	resp, err := c.doRequest(ctx, http.MethodGet, c.getURL("/process/%s/logs/%d", id, count), nil)
	if err != nil {
		return jasper.LogStream{}, err
	}
	defer resp.Body.Close()

	stream := jasper.LogStream{}
	if err = gimlet.GetJSON(resp.Body, &stream); err != nil {
		return jasper.LogStream{}, errors.Wrap(err, "problem reading logs from response")
	}

	return stream, nil
}

func (c *restClient) DownloadFile(ctx context.Context, opts options.Download) error {
	body, err := makeBody(opts)
	if err != nil {
		return errors.Wrap(err, "problem building request")
	}

	resp, err := c.doRequest(ctx, http.MethodPost, c.getURL("/download"), body)
	if err != nil {
		return errors.Wrap(err, "problem downloading file")
	}
	defer resp.Body.Close()

	return nil
}

// DownloadMongoDB downloads the desired version of MongoDB.
func (c *restClient) DownloadMongoDB(ctx context.Context, opts options.MongoDBDownload) error {
	body, err := makeBody(opts)
	if err != nil {
		return errors.Wrap(err, "building request")
	}

	resp, err := c.doRequest(ctx, http.MethodPost, c.getURL("/download/mongodb"), body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// ConfigureCache changes the cache configurations.
func (c *restClient) ConfigureCache(ctx context.Context, opts options.Cache) error {
	body, err := makeBody(opts)
	if err != nil {
		return errors.Wrap(err, "building request")
	}

	resp, err := c.doRequest(ctx, http.MethodPost, c.getURL("/download/cache"), body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (c *restClient) SignalEvent(ctx context.Context, name string) error {
	resp, err := c.doRequest(ctx, http.MethodPatch, c.getURL("/signal/event/%s", name), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (c *restClient) WriteFile(ctx context.Context, opts options.WriteFile) error {
	sendOpts := func(opts options.WriteFile) error {
		body, err := makeBody(opts)
		if err != nil {
			return errors.Wrap(err, "problem building request")
		}
		resp, err := c.doRequest(ctx, http.MethodPut, c.getURL("/file/write"), body)
		if err != nil {
			return errors.Wrap(err, "problem writing file")
		}
		return errors.Wrap(resp.Body.Close(), "problem closing response body")
	}

	return opts.WriteBufferedContent(sendOpts)
}

func (c *restClient) SendMessages(ctx context.Context, lp options.LoggingPayload) error {
	body, err := makeBody(lp)
	if err != nil {
		return errors.Wrap(err, "building request")
	}

	resp, err := c.doRequest(ctx, http.MethodPost, c.getURL("/logging/id/%s/send", lp.LoggerID), body)
	if err != nil {
		return errors.Wrap(err, "request returned error")
	}
	defer resp.Body.Close()

	if err = handleError(resp); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (c *restClient) LoggingCache(ctx context.Context) jasper.LoggingCache {
	return &restLoggingCache{
		client: c,
		ctx:    ctx,
	}
}
