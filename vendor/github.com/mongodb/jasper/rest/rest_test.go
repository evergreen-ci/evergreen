package rest

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
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

	tempDir, err := ioutil.TempDir("./", "jasper-rest-service-test")
	require.NoError(t, err)
	defer func() { require.NoError(t, os.RemoveAll(tempDir)) }()

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
		"EmptyCreateOpts": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			proc, err := client.CreateProcess(ctx, &options.Create{})
			assert.Error(t, err)
			assert.Nil(t, proc)
		},
		"WithOnlyTimeoutValue": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			proc, err := client.CreateProcess(ctx, &options.Create{Args: []string{"ls"}, TimeoutSecs: 300})
			assert.NoError(t, err)
			assert.NotNil(t, proc)
		},
		"ListErrorsWithInvalidFilter": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			list, err := client.List(ctx, "foo")
			assert.Error(t, err)
			assert.Nil(t, list)
		},
		"RegisterAlwaysErrors": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			proc, err := client.CreateProcess(ctx, &options.Create{Args: []string{"ls"}})
			assert.NotNil(t, proc)
			require.NoError(t, err)

			assert.Error(t, client.Register(ctx, nil))
			assert.Error(t, client.Register(nil, nil))
			assert.Error(t, client.Register(ctx, proc))
			assert.Error(t, client.Register(nil, proc))
		},
		"ClientMethodsErrorWithBadUrl": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
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
		"ProcessMethodsWithBadUrl": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
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
		"StandardInput": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			for subTestName, subTestCase := range map[string]func(ctx context.Context, t *testing.T, opts *options.Create, expectedOutput string, stdin []byte){
				"ReaderIsIgnored": func(ctx context.Context, t *testing.T, opts *options.Create, expectedOutput string, stdin []byte) {
					opts.StandardInput = bytes.NewBuffer(stdin)

					proc, err := client.CreateProcess(ctx, opts)
					require.NoError(t, err)

					_, err = proc.Wait(ctx)
					require.NoError(t, err)

					logs, err := client.GetLogStream(ctx, proc.ID(), 1)
					require.NoError(t, err)
					assert.Empty(t, logs.Logs)
				},
				"BytesSetsStandardInput": func(ctx context.Context, t *testing.T, opts *options.Create, expectedOutput string, stdin []byte) {
					opts.StandardInputBytes = stdin

					proc, err := client.CreateProcess(ctx, opts)
					require.NoError(t, err)

					_, err = proc.Wait(ctx)
					require.NoError(t, err)

					logs, err := client.GetLogStream(ctx, proc.ID(), 1)
					require.NoError(t, err)

					require.Len(t, logs.Logs, 1)
					assert.Equal(t, expectedOutput, strings.TrimSpace(logs.Logs[0]))
				},
				"BytesCopiedByRespawnedProcess": func(ctx context.Context, t *testing.T, opts *options.Create, expectedOutput string, stdin []byte) {
					opts.StandardInputBytes = stdin

					proc, err := client.CreateProcess(ctx, opts)
					require.NoError(t, err)

					_, err = proc.Wait(ctx)
					require.NoError(t, err)

					logs, err := client.GetLogStream(ctx, proc.ID(), 1)
					require.NoError(t, err)

					require.Len(t, logs.Logs, 1)
					assert.Equal(t, expectedOutput, strings.TrimSpace(logs.Logs[0]))

					newProc, err := proc.Respawn(ctx)
					require.NoError(t, err)

					_, err = newProc.Wait(ctx)
					require.NoError(t, err)

					logs, err = client.GetLogStream(ctx, newProc.ID(), 1)
					require.NoError(t, err)

					require.Len(t, logs.Logs, 1)
					assert.Equal(t, expectedOutput, strings.TrimSpace(logs.Logs[0]))
				},
			} {
				t.Run(subTestName, func(t *testing.T) {
					opts := &options.Create{
						Args: []string{"bash", "-s"},
						Output: options.Output{
							Loggers: []options.Logger{jasper.NewInMemoryLogger(1)},
						},
					}
					expectedOutput := "foobar"
					stdin := []byte("echo " + expectedOutput)
					subTestCase(ctx, t, opts, expectedOutput, stdin)
				})
			}
		},
		"InvalidFilterReturnsError": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			procs, err := client.List(ctx, options.Filter("foo"))
			assert.Error(t, err)
			assert.Nil(t, procs)
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
			assert.Contains(t, err.Error(), "problem managing resources")
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
		"SignalFailsToParsePid": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			req, err := http.NewRequest(http.MethodPatch, client.getURL("/process/%s/signal/f", "foo"), nil)
			require.NoError(t, err)
			req = req.WithContext(ctx)

			resp, err := client.client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			assert.Contains(t, handleError(resp).Error(), "problem converting signal 'f'")
		},
		"DownloadFileCreatesResource": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			file, err := ioutil.TempFile(tempDir, "out.txt")
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, file.Close())
				assert.NoError(t, os.RemoveAll(file.Name()))
			}()
			absPath, err := filepath.Abs(file.Name())
			require.NoError(t, err)

			assert.NoError(t, client.DownloadFile(ctx, options.Download{URL: "https://example.com", Path: absPath}))

			info, err := os.Stat(file.Name())
			assert.NoError(t, err)
			assert.NotEqual(t, 0, info.Size())
		},
		"DownloadFileCreatesResourceAndExtracts": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			downloadDir, err := ioutil.TempDir(tempDir, "rest_test")
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, os.RemoveAll(downloadDir))
			}()

			fileServerDir, err := ioutil.TempDir(tempDir, "rest_test_server")
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, os.RemoveAll(fileServerDir))
			}()

			fileName := "foo.zip"
			fileContents := "foo"
			require.NoError(t, testutil.AddFileToDirectory(fileServerDir, fileName, fileContents))

			absDownloadDir, err := filepath.Abs(downloadDir)
			require.NoError(t, err)
			destFilePath := filepath.Join(absDownloadDir, fileName)
			destExtractDir := filepath.Join(absDownloadDir, "extracted")

			port := testutil.GetPortNumber()
			fileServerAddr := fmt.Sprintf("localhost:%d", port)
			fileServer := &http.Server{Addr: fileServerAddr, Handler: http.FileServer(http.Dir(fileServerDir))}
			defer func() {
				assert.NoError(t, fileServer.Close())
			}()
			listener, err := net.Listen("tcp", fileServerAddr)
			require.NoError(t, err)
			go func() {
				fileServer.Serve(listener)
			}()

			baseURL := fmt.Sprintf("http://%s", fileServerAddr)
			require.NoError(t, testutil.WaitForRESTService(ctx, baseURL))

			info := options.Download{
				URL:  fmt.Sprintf("%s/%s", baseURL, fileName),
				Path: destFilePath,
				ArchiveOpts: options.Archive{
					ShouldExtract: true,
					Format:        options.ArchiveZip,
					TargetPath:    destExtractDir,
				},
			}
			require.NoError(t, client.DownloadFile(ctx, info))

			fileInfo, err := os.Stat(destFilePath)
			require.NoError(t, err)
			assert.NotZero(t, fileInfo.Size())

			dirContents, err := ioutil.ReadDir(destExtractDir)
			require.NoError(t, err)

			assert.NotZero(t, len(dirContents))
		},
		"DownloadFileFailsExtractionWithInvalidArchiveFormat": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			fileName := filepath.Join(tempDir, "out.txt")
			_, err := os.Stat(fileName)
			require.True(t, os.IsNotExist(err))

			info := options.Download{
				URL:  "https://example.com",
				Path: fileName,
				ArchiveOpts: options.Archive{
					ShouldExtract: true,
					Format:        options.ArchiveFormat("foo"),
				},
			}
			assert.Error(t, client.DownloadFile(ctx, info))

			_, err = os.Stat(fileName)
			assert.True(t, os.IsNotExist(err))
		},
		"DownloadFileFailsForUnarchivedFile": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			file, err := ioutil.TempFile(tempDir, "out.txt")
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, file.Close())
				assert.NoError(t, os.RemoveAll(file.Name()))
			}()
			extractDir, err := ioutil.TempDir(tempDir, "out")
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, os.RemoveAll(extractDir))
			}()

			info := options.Download{
				URL:  "https://example.com",
				Path: file.Name(),
				ArchiveOpts: options.Archive{
					ShouldExtract: true,
					Format:        options.ArchiveAuto,
				},
			}
			assert.Error(t, client.DownloadFile(ctx, info))

			dirContents, err := ioutil.ReadDir(extractDir)
			require.NoError(t, err)
			assert.Zero(t, len(dirContents))
		},
		"DownloadFileFailsWithInvalidURL": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			err := client.DownloadFile(ctx, options.Download{URL: "", Path: ""})
			assert.Error(t, err)
		},
		"DownloadFileFailsForNonexistentURL": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			file, err := ioutil.TempFile(tempDir, "out.txt")
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, file.Close())
				assert.NoError(t, os.RemoveAll(file.Name()))
			}()
			assert.Error(t, client.DownloadFile(ctx, options.Download{URL: "https://example.com/foo", Path: file.Name()}))
		},
		"DownloadFileFailsForInsufficientPermissions": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			if os.Geteuid() == 0 {
				t.Skip("cannot test download permissions as root")
			} else if runtime.GOOS == "windows" {
				t.Skip("cannot test download permissions on windows")
			}
			assert.Error(t, client.DownloadFile(ctx, options.Download{URL: "https://example.com", Path: "/foo/bar"}))
		},
		"ServiceDownloadFileFailsWithInvalidInfo": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
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
		"ServiceDownloadFileFailsWithInvalidURL": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			fileName := filepath.Join(tempDir, "out.txt")
			absPath, err := filepath.Abs(fileName)
			require.NoError(t, err)

			body, err := makeBody(options.Download{
				URL:  "://example.com",
				Path: absPath,
			})
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, client.getURL("/download"), body)
			require.NoError(t, err)
			rw := httptest.NewRecorder()

			srv.downloadFile(rw, req)
			assert.Equal(t, http.StatusInternalServerError, rw.Code)
		},
		"WithInMemoryLogger": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			output := "foo"
			opts := &options.Create{
				Args: []string{"echo", output},
				Output: options.Output{
					Loggers: []options.Logger{
						{
							Type:    options.LogInMemory,
							Options: options.Log{InMemoryCap: 100, Format: options.LogFormatPlain},
						},
					},
				},
			}

			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, proc jasper.Process){
				"GetLogStreamFailsForInvalidCount": func(ctx context.Context, t *testing.T, proc jasper.Process) {
					stream, err := client.GetLogStream(ctx, proc.ID(), -1)
					assert.Error(t, err)
					assert.Zero(t, stream)
				},
				"GetLogStreamReturnsOutputOnSuccess": func(ctx context.Context, t *testing.T, proc jasper.Process) {
					logs := []string{}
					for stream, err := client.GetLogStream(ctx, proc.ID(), 1); !stream.Done; stream, err = client.GetLogStream(ctx, proc.ID(), 1) {
						require.NoError(t, err)
						require.NotEmpty(t, stream.Logs)
						logs = append(logs, stream.Logs...)
					}
					assert.Contains(t, logs, output)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					proc, err := client.CreateProcess(ctx, opts)
					require.NoError(t, err)
					require.NotNil(t, proc)

					_, err = proc.Wait(ctx)
					require.NoError(t, err)
					testCase(ctx, t, proc)
				})
			}
		},
		"GetLogStreamFromNonexistentProcessFails": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			stream, err := client.GetLogStream(ctx, "foo", 1)
			assert.Error(t, err)
			assert.Zero(t, stream)
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
		"GetBuildloggerURLsFailsWithNonexistentProcess": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			urls, err := client.GetBuildloggerURLs(ctx, "foo")
			assert.Error(t, err)
			assert.Nil(t, urls)
		},
		"GetBuildloggerURLsFailsWithoutBuildlogger": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			opts := &options.Create{Args: []string{"echo", "foo"}}
			opts.Output.Loggers = []options.Logger{{Type: options.LogDefault, Options: options.Log{Format: options.LogFormatPlain}}}

			proc, err := client.CreateProcess(ctx, opts)
			assert.NoError(t, err)
			assert.NotNil(t, proc)

			urls, err := client.GetBuildloggerURLs(ctx, proc.ID())
			assert.Error(t, err)
			assert.Nil(t, urls)
		},
		"CreateWithMultipleLoggers": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			file, err := ioutil.TempFile(tempDir, "out.txt")
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, file.Close())
				assert.NoError(t, os.RemoveAll(file.Name()))
			}()

			fileLogger := options.Logger{
				Type: options.LogFile,
				Options: options.Log{
					FileName: file.Name(),
					Format:   options.LogFormatPlain,
				},
			}

			inMemoryLogger := options.Logger{
				Type: options.LogInMemory,
				Options: options.Log{
					Format:      options.LogFormatPlain,
					InMemoryCap: 100,
				},
			}

			opts := &options.Create{Output: options.Output{Loggers: []options.Logger{inMemoryLogger, fileLogger}}}
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

			info := options.WriteFile{Path: tmpFile.Name(), Content: []byte("foo")}
			require.NoError(t, client.WriteFile(ctx, info))

			content, err := ioutil.ReadFile(tmpFile.Name())
			require.NoError(t, err)

			assert.Equal(t, info.Content, content)
		},
		"WriteFileAcceptsContentFromReader": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			tmpFile, err := ioutil.TempFile(tempDir, filepath.Base(t.Name()))
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, tmpFile.Close())
				assert.NoError(t, os.RemoveAll(tmpFile.Name()))
			}()

			buf := []byte("foo")
			info := options.WriteFile{Path: tmpFile.Name(), Reader: bytes.NewBuffer(buf)}
			require.NoError(t, client.WriteFile(ctx, info))

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
			info := options.WriteFile{Path: tmpFile.Name(), Reader: bytes.NewBuffer(buf)}
			require.NoError(t, client.WriteFile(ctx, info))

			content, err := ioutil.ReadFile(tmpFile.Name())
			require.NoError(t, err)

			assert.Equal(t, buf, content)
		},
		"WriteFileFailsWithInvalidPath": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			info := options.WriteFile{Content: []byte("foo")}
			assert.Error(t, client.WriteFile(ctx, info))
		},
		"WriteFileSucceedsWithNoContent": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			path := filepath.Join(tempDir, "write_file")
			require.NoError(t, os.RemoveAll(path))
			defer func() {
				assert.NoError(t, os.RemoveAll(path))
			}()

			info := options.WriteFile{Path: path}
			require.NoError(t, client.WriteFile(ctx, info))

			stat, err := os.Stat(path)
			require.NoError(t, err)

			assert.Zero(t, stat.Size())
		},
		"ServiceRegisterSignalTriggerIDChecksForExistingProcess": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			req, err := http.NewRequest(http.MethodPatch, client.getURL("/process/%s/trigger/signal/%s", "foo", jasper.CleanTerminationSignalTrigger), nil)
			require.NoError(t, err)

			resp, err := client.client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			assert.Contains(t, handleError(resp).Error(), "no process 'foo' found")
		},
		"ServiceRegisterSignalTriggerIDChecksForInvalidTriggerID": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			proc, err := client.CreateProcess(ctx, testutil.YesCreateOpts(0))
			require.NoError(t, err)
			assert.True(t, proc.Running(ctx))

			assert.Error(t, proc.RegisterSignalTriggerID(ctx, jasper.SignalTriggerID("foo")))

			assert.NoError(t, proc.Signal(ctx, syscall.SIGTERM))
		},
		"ServiceRegisterSignalTriggerIDPassesWithValidArgs": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			proc, err := client.CreateProcess(ctx, testutil.YesCreateOpts(0))
			require.NoError(t, err)
			assert.True(t, proc.Running(ctx))

			assert.NoError(t, proc.RegisterSignalTriggerID(ctx, jasper.CleanTerminationSignalTrigger))

			assert.NoError(t, proc.Signal(ctx, syscall.SIGTERM))
		},
		// "": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {},
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
