package remote

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/mock"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type neverJSON struct{}

func (n *neverJSON) MarshalJSON() ([]byte, error)  { return nil, errors.New("always error") }
func (n *neverJSON) UnmarshalJSON(in []byte) error { return errors.New("always error") }
func (n *neverJSON) Read(p []byte) (int, error)    { return 0, errors.New("always error") }
func (n *neverJSON) Close() error                  { return errors.New("always error") }

func TestRestService(t *testing.T) {
	httpClient := testutil.GetHTTPClient()
	defer testutil.PutHTTPClient(httpClient)

	tempDir, err := ioutil.TempDir(testutil.BuildDirectory(), filepath.Base(t.Name()))
	require.NoError(t, err)
	defer func() { assert.NoError(t, os.RemoveAll(tempDir)) }()

	for name, test := range map[string]func(context.Context, *testing.T, *Service, *restClient){
		"VerifyFixtures": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			assert.NotNil(t, srv)
			assert.NotNil(t, client)
			assert.NotNil(t, srv.manager)
			assert.NotNil(t, client.client)
			assert.NotZero(t, client.prefix)

			// similarly about helper functions
			client.prefix = ""
			assert.Equal(t, "/foo", client.getURL("foo"))
			_, err := makeBody(&neverJSON{})
			assert.Error(t, err)
			assert.Error(t, handleError(&http.Response{Body: &neverJSON{}, StatusCode: http.StatusTeapot}))
		},
		"ClientMethodsErrorWithBadURL": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			client.prefix = strings.Replace(client.prefix, "http://", "://", 1)

			_, err := client.List(ctx, options.All)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem building request")

			_, err = client.CreateProcess(ctx, nil)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem building request")

			_, err = client.Group(ctx, "foo")
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem building request")

			_, err = client.Get(ctx, "foo")
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem building request")

			err = client.Close(ctx)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem building request")

			_, err = client.getProcessInfo(ctx, "foo")
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem building request")

			_, err = client.GetLogStream(ctx, "foo", 1)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem building request")

			_, err = client.GetBuildloggerURLs(ctx, "foo")
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem building request")

			err = client.DownloadFile(ctx, options.Download{URL: "foo", Path: "bar"})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem building request")

			err = client.DownloadMongoDB(ctx, options.MongoDBDownload{})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem building request")

			err = client.ConfigureCache(ctx, options.Cache{})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem building request")
		},
		"ClientRequestsFailWithMalformedURL": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			client.prefix = strings.Replace(client.prefix, "http://", "http;//", 1)

			_, err := client.List(ctx, options.All)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem making request")

			_, err = client.Group(ctx, "foo")
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem making request")

			_, err = client.CreateProcess(ctx, nil)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem making request")

			_, err = client.Get(ctx, "foo")
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem making request")

			err = client.Close(ctx)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem making request")

			_, err = client.getProcessInfo(ctx, "foo")
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem making request")

			_, err = client.GetLogStream(ctx, "foo", 1)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem making request")

			_, err = client.GetBuildloggerURLs(ctx, "foo")
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem making request")

			err = client.DownloadFile(ctx, options.Download{URL: "foo", Path: "bar"})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem making request")

			err = client.DownloadMongoDB(ctx, options.MongoDBDownload{})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem making request")

			err = client.ConfigureCache(ctx, options.Cache{})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem making request")
		},
		"ProcessMethodsWithBadURL": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			client.prefix = strings.Replace(client.prefix, "http://", "://", 1)

			proc := &restProcess{
				client: client,
				id:     "foo",
			}

			err := proc.Signal(ctx, syscall.SIGTERM)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem building request")

			_, err = proc.Wait(ctx)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem building request")

			proc.Tag("a")

			out := proc.GetTags()
			assert.Nil(t, out)

			proc.ResetTags()

			err = proc.RegisterSignalTriggerID(ctx, jasper.CleanTerminationSignalTrigger)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem building request")
		},
		"ProcessRequestsFailWithBadURL": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {

			client.prefix = strings.Replace(client.prefix, "http://", "http;//", 1)

			proc := &restProcess{
				client: client,
				id:     "foo",
			}

			err := proc.Signal(ctx, syscall.SIGTERM)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem making request")

			_, err = proc.Wait(ctx)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem making request")

			proc.Tag("a")

			out := proc.GetTags()
			assert.Nil(t, out)

			proc.ResetTags()

			err = proc.RegisterSignalTriggerID(ctx, jasper.CleanTerminationSignalTrigger)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem making request")
		},
		"CheckSafetyOfTagMethodsForBrokenTasks": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			proc := &restProcess{
				client: client,
				id:     "foo",
			}

			proc.Tag("a")

			out := proc.GetTags()
			assert.Nil(t, out)

			proc.ResetTags()
		},
		"SignalFailsForTaskThatDoesNotExist": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			proc := &restProcess{
				client: client,
				id:     "foo",
			}

			err := proc.Signal(ctx, syscall.SIGTERM)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "no process")
		},
		"CreateProcessEndpointErrorsWithMalformedData": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			body, err := makeBody(map[string]int{"tags": 42})
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, "", ioutil.NopCloser(body))
			require.NoError(t, err)
			rw := httptest.NewRecorder()
			srv.createProcess(rw, req)
			assert.Equal(t, http.StatusBadRequest, rw.Code)
		},
		"WaitForProcessThatDoesNotExistShouldError": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			proc := &restProcess{
				client: client,
				id:     "foo",
			}

			_, err := proc.Wait(ctx)
			assert.Error(t, err)
		},
		"SignalProcessThatDoesNotExistShouldError": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			proc := &restProcess{
				client: client,
				id:     "foo",
			}

			assert.Error(t, proc.Signal(ctx, syscall.SIGTERM))
		},
		"SignalErrorsWithInvalidSyscall": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			proc, err := client.CreateProcess(ctx, testutil.SleepCreateOpts(10))
			require.NoError(t, err)

			assert.Error(t, proc.Signal(ctx, syscall.Signal(-1)))
		},
		"MetricsErrorForInvalidProcess": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			req, err := http.NewRequest(http.MethodGet, client.getURL("/process/%s/metrics", "foo"), nil)
			require.NoError(t, err)
			req = req.WithContext(ctx)
			res, err := httpClient.Do(req)
			require.NoError(t, err)

			assert.Equal(t, http.StatusNotFound, res.StatusCode)
		},
		"GetProcessWhenInvalid": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			_, err := client.Get(ctx, "foo")
			assert.Error(t, err)
		},
		"CreateFailPropagatesErrors": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			srv.manager = &mock.Manager{FailCreate: true}
			proc, err := client.CreateProcess(ctx, testutil.TrueCreateOpts())
			assert.Error(t, err)
			assert.Nil(t, proc)
			assert.Contains(t, err.Error(), "problem submitting request")
		},
		"CreateFailsForTriggerReasons": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			srv.manager = &mock.Manager{
				CreateConfig: mock.Process{FailRegisterTrigger: true},
			}
			proc, err := client.CreateProcess(ctx, testutil.TrueCreateOpts())
			require.Error(t, err)
			assert.Nil(t, proc)
			assert.Contains(t, err.Error(), "problem registering trigger")
		},
		"MetricsPopulatedForValidProcess": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			id := "foo"
			srv.manager = &mock.Manager{
				Procs: []jasper.Process{
					&mock.Process{ProcInfo: jasper.ProcessInfo{ID: id, PID: os.Getpid()}},
				},
			}

			req, err := http.NewRequest(http.MethodGet, client.getURL("/process/%s/metrics", id), nil)
			require.NoError(t, err)
			req = req.WithContext(ctx)
			res, err := httpClient.Do(req)
			require.NoError(t, err)

			assert.Equal(t, http.StatusOK, res.StatusCode)
		},
		"AddTagsWithNoTagsSpecifiedShouldError": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			id := "foo"
			srv.manager = &mock.Manager{
				Procs: []jasper.Process{
					&mock.Process{ProcInfo: jasper.ProcessInfo{ID: id}},
				},
			}

			req, err := http.NewRequest(http.MethodPost, client.getURL("/process/%s/tags", id), nil)
			require.NoError(t, err)
			req = req.WithContext(ctx)
			res, err := httpClient.Do(req)
			require.NoError(t, err)

			assert.Equal(t, http.StatusBadRequest, res.StatusCode)

		},
		"SignalInPassingCase": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			id := "foo"
			srv.manager = &mock.Manager{
				Procs: []jasper.Process{
					&mock.Process{ProcInfo: jasper.ProcessInfo{ID: id}},
				},
			}
			proc := &restProcess{
				client: client,
				id:     id,
			}

			err := proc.Signal(ctx, syscall.SIGTERM)
			assert.NoError(t, err)

		},
		"SignalFailsToParsePID": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			req, err := http.NewRequest(http.MethodPatch, client.getURL("/process/%s/signal/f", "foo"), nil)
			require.NoError(t, err)
			req = req.WithContext(ctx)

			resp, err := client.client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			assert.Contains(t, handleError(resp).Error(), "problem converting signal 'f'")
		},
		"ServiceDownloadFileFailsWithInvalidOptions": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			body, err := makeBody(struct {
				URL int `json:"url"`
			}{URL: 0})
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, client.getURL("/download"), body)
			require.NoError(t, err)
			rw := httptest.NewRecorder()
			srv.downloadFile(rw, req)
			assert.Equal(t, http.StatusBadRequest, rw.Code)
		},
		"GetLogStreamFailsWithoutInMemoryLogger": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			opts := &options.Create{Args: []string{"echo", "foo"}}

			proc, err := client.CreateProcess(ctx, opts)
			require.NoError(t, err)
			require.NotNil(t, proc)

			_, err = proc.Wait(ctx)
			require.NoError(t, err)

			stream, err := client.GetLogStream(ctx, proc.ID(), 1)
			assert.Error(t, err)
			assert.Zero(t, stream)
		},
		"InitialCacheOptionsMatchDefault": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			assert.Equal(t, jasper.DefaultMaxCacheSize, srv.cacheOpts.MaxSize)
			assert.Equal(t, jasper.DefaultCachePruneDelay, srv.cacheOpts.PruneDelay)
			assert.Equal(t, false, srv.cacheOpts.Disabled)
		},
		"ConfigureCacheFailsWithInvalidOptions": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			opts := options.Cache{PruneDelay: -1}
			assert.Error(t, client.ConfigureCache(ctx, opts))
			assert.Equal(t, jasper.DefaultMaxCacheSize, srv.cacheOpts.MaxSize)
			assert.Equal(t, jasper.DefaultCachePruneDelay, srv.cacheOpts.PruneDelay)

			opts = options.Cache{MaxSize: -1}
			assert.Error(t, client.ConfigureCache(ctx, opts))
			assert.Equal(t, jasper.DefaultMaxCacheSize, srv.cacheOpts.MaxSize)
			assert.Equal(t, jasper.DefaultCachePruneDelay, srv.cacheOpts.PruneDelay)
		},
		"ConfigureCachePassesWithZeroOptions": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			opts := options.Cache{}
			assert.NoError(t, client.ConfigureCache(ctx, opts))
		},
		"ConfigureCachePassesWithValidOptions": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			opts := options.Cache{PruneDelay: 5 * time.Second, MaxSize: 1024}
			assert.NoError(t, client.ConfigureCache(ctx, opts))
		},
		"ConfigureCacheFailsWithBadRequest": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			var opts struct {
				MaxSize string `json:"max_size"`
			}
			opts.MaxSize = "foo"
			body, err := makeBody(opts)
			require.NoError(t, err)
			req, err := http.NewRequest(http.MethodPost, client.getURL("/configure-cache"), body)
			require.NoError(t, err)
			rw := httptest.NewRecorder()

			srv.configureCache(rw, req)
			assert.Equal(t, http.StatusBadRequest, rw.Code)
		},
		"DownloadMongoDBFailsWithBadRequest": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			var opts struct {
				BuildOpts string
			}
			body, err := makeBody(opts)
			require.NoError(t, err)
			req, err := http.NewRequest(http.MethodPost, client.getURL("/download-mongodb"), body)
			require.NoError(t, err)
			rw := httptest.NewRecorder()

			srv.downloadMongoDB(rw, req)
			assert.Equal(t, http.StatusBadRequest, rw.Code)
		},
		"DownloadMongoDBFailsWithZeroOptions": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			err := client.DownloadMongoDB(ctx, options.MongoDBDownload{})
			assert.Error(t, err)
		},
		"DownloadMongoDBPassesWithValidOptions": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			dir, err := ioutil.TempDir(tempDir, "mongodb")
			require.NoError(t, err)
			defer os.RemoveAll(dir)
			absDir, err := filepath.Abs(dir)
			require.NoError(t, err)

			opts := testutil.ValidMongoDBDownloadOptions()
			opts.Path = absDir

			err = client.DownloadMongoDB(ctx, opts)
			assert.NoError(t, err)
		},
		"CreateWithMultipleLoggers": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			file, err := ioutil.TempFile(tempDir, "out.txt")
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, file.Close())
				assert.NoError(t, os.RemoveAll(file.Name()))
			}()

			fileLogger := &options.LoggerConfig{}
			require.NoError(t, fileLogger.Set(&options.FileLoggerOptions{
				Filename: file.Name(),
				Base:     options.BaseOptions{Format: options.LogFormatPlain},
			}))

			inMemoryLogger := &options.LoggerConfig{}
			require.NoError(t, inMemoryLogger.Set(&options.InMemoryLoggerOptions{
				InMemoryCap: 100,
				Base:        options.BaseOptions{Format: options.LogFormatPlain},
			}))

			opts := &options.Create{Output: options.Output{Loggers: []*options.LoggerConfig{inMemoryLogger, fileLogger}}}
			opts.Args = []string{"echo", "foobar"}
			proc, err := client.CreateProcess(ctx, opts)
			require.NoError(t, err)
			_, err = proc.Wait(ctx)
			require.NoError(t, err)

			stream, err := client.GetLogStream(ctx, proc.ID(), 1)
			require.NoError(t, err)
			assert.NotEmpty(t, stream.Logs)
			assert.False(t, stream.Done)

			info, err := os.Stat(file.Name())
			require.NoError(t, err)
			assert.NotZero(t, info.Size())

		},
		"WriteFileSucceeds": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			tmpFile, err := ioutil.TempFile(tempDir, filepath.Base(t.Name()))
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, tmpFile.Close())
				assert.NoError(t, os.RemoveAll(tmpFile.Name()))
			}()

			opts := options.WriteFile{Path: tmpFile.Name(), Content: []byte("foo")}
			require.NoError(t, client.WriteFile(ctx, opts))

			content, err := ioutil.ReadFile(tmpFile.Name())
			require.NoError(t, err)

			assert.Equal(t, opts.Content, content)
		},
		"WriteFileAcceptsContentFromReader": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			tmpFile, err := ioutil.TempFile(tempDir, filepath.Base(t.Name()))
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, tmpFile.Close())
				assert.NoError(t, os.RemoveAll(tmpFile.Name()))
			}()

			buf := []byte("foo")
			opts := options.WriteFile{Path: tmpFile.Name(), Reader: bytes.NewBuffer(buf)}
			require.NoError(t, client.WriteFile(ctx, opts))

			content, err := ioutil.ReadFile(tmpFile.Name())
			require.NoError(t, err)

			assert.Equal(t, buf, content)
		},
		"WriteFileSucceedsWithLargeContentReader": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			tmpFile, err := ioutil.TempFile(tempDir, filepath.Base(t.Name()))
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, tmpFile.Close())
				assert.NoError(t, os.RemoveAll(tmpFile.Name()))
			}()

			const mb = 1024 * 1024
			buf := bytes.Repeat([]byte("foo"), 2*mb)
			opts := options.WriteFile{Path: tmpFile.Name(), Reader: bytes.NewBuffer(buf)}
			require.NoError(t, client.WriteFile(ctx, opts))

			content, err := ioutil.ReadFile(tmpFile.Name())
			require.NoError(t, err)

			assert.Equal(t, buf, content)
		},
		"RegisterSignalTriggerIDChecksForExistingProcess": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			req, err := http.NewRequest(http.MethodPatch, client.getURL("/process/%s/trigger/signal/%s", "foo", jasper.CleanTerminationSignalTrigger), nil)
			require.NoError(t, err)

			resp, err := client.client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			assert.Contains(t, handleError(resp).Error(), "no process 'foo' found")
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), testutil.LongTestTimeout)
			defer cancel()

			srv, port, err := startRESTService(ctx, httpClient)
			require.NoError(t, err)
			require.NotNil(t, srv)

			require.NoError(t, err)
			client := &restClient{
				prefix: fmt.Sprintf("http://localhost:%d/jasper/v1", port),
				client: httpClient,
			}

			test(ctx, t, srv, client)
		})
	}
}
