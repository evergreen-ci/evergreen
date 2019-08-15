package jasper

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
	httpClient := GetHTTPClient()
	defer PutHTTPClient(httpClient)

	for name, test := range map[string]func(context.Context, *testing.T, *Service, *restClient){
		"VerifyFixtures": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			assert.NotNil(t, srv)
			assert.NotNil(t, client)
			assert.NotNil(t, srv.manager)
			assert.NotNil(t, client.client)
			assert.NotZero(t, client.prefix)

			// no good other place to put this assertion
			// about the constructor
			newm := NewManagerService(&basicProcessManager{})
			assert.IsType(t, &localProcessManager{}, srv.manager)
			assert.IsType(t, &localProcessManager{}, newm.manager)

			// similarly about helper functions
			client.prefix = ""
			assert.Equal(t, "/foo", client.getURL("foo"))
			_, err := makeBody(&neverJSON{})
			assert.Error(t, err)
			assert.Error(t, handleError(&http.Response{Body: &neverJSON{}, StatusCode: http.StatusTeapot}))
		},
		"EmptyCreateOpts": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			proc, err := client.CreateProcess(ctx, &CreateOptions{})
			assert.Error(t, err)
			assert.Nil(t, proc)
		},
		"WithOnlyTimeoutValue": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			proc, err := client.CreateProcess(ctx, &CreateOptions{Args: []string{"ls"}, TimeoutSecs: 300})
			assert.NoError(t, err)
			assert.NotNil(t, proc)
		},
		"ListErrorsWithInvalidFilter": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			list, err := client.List(ctx, "foo")
			assert.Error(t, err)
			assert.Nil(t, list)
		},
		"RegisterAlwaysErrors": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			proc, err := newBlockingProcess(ctx, trueCreateOpts())
			require.NoError(t, err)

			assert.Error(t, client.Register(ctx, nil))
			assert.Error(t, client.Register(nil, nil))
			assert.Error(t, client.Register(ctx, proc))
			assert.Error(t, client.Register(nil, proc))
		},
		"ClientMethodsErrorWithBadUrl": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			client.prefix = strings.Replace(client.prefix, "http://", "://", 1)

			_, err := client.List(ctx, All)
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

			err = client.DownloadFile(ctx, DownloadInfo{URL: "foo", Path: "bar"})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem building request")

			err = client.DownloadMongoDB(ctx, MongoDBDownloadOptions{})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem building request")

			err = client.ConfigureCache(ctx, CacheOptions{})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem building request")
		},
		"ClientRequestsFailWithMalformedURL": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			client.prefix = strings.Replace(client.prefix, "http://", "http;//", 1)

			_, err := client.List(ctx, All)
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

			err = client.DownloadFile(ctx, DownloadInfo{URL: "foo", Path: "bar"})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem making request")

			err = client.DownloadMongoDB(ctx, MongoDBDownloadOptions{})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "problem making request")

			err = client.ConfigureCache(ctx, CacheOptions{})
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

			err = proc.RegisterSignalTriggerID(ctx, CleanTerminationSignalTrigger)
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

			err = proc.RegisterSignalTriggerID(ctx, CleanTerminationSignalTrigger)
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
		"CreateFailPropagatesErrors": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			srv.manager = &MockManager{FailCreate: true}
			proc, err := client.CreateProcess(ctx, trueCreateOpts())
			assert.Error(t, err)
			assert.Nil(t, proc)
			assert.Contains(t, err.Error(), "problem submitting request")
		},
		"CreateFailsForTriggerReasons": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			srv.manager = &MockManager{
				CreateConfig: MockProcess{FailRegisterTrigger: true},
			}
			proc, err := client.CreateProcess(ctx, trueCreateOpts())
			require.Error(t, err)
			assert.Nil(t, proc)
			assert.Contains(t, err.Error(), "problem managing resources")
		},
		"StandardInput": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			for subTestName, subTestCase := range map[string]func(ctx context.Context, t *testing.T, opts *CreateOptions, expectedOutput string, stdin []byte){
				"ReaderIsIgnored": func(ctx context.Context, t *testing.T, opts *CreateOptions, expectedOutput string, stdin []byte) {
					opts.StandardInput = bytes.NewBuffer(stdin)

					proc, err := client.CreateProcess(ctx, opts)
					require.NoError(t, err)

					_, err = proc.Wait(ctx)
					require.NoError(t, err)

					logs, err := client.GetLogStream(ctx, proc.ID(), 1)
					require.NoError(t, err)
					assert.Empty(t, logs.Logs)
				},
				"BytesSetsStandardInput": func(ctx context.Context, t *testing.T, opts *CreateOptions, expectedOutput string, stdin []byte) {
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
				"BytesCopiedByRespawnedProcess": func(ctx context.Context, t *testing.T, opts *CreateOptions, expectedOutput string, stdin []byte) {
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
					opts := &CreateOptions{
						Args: []string{"bash", "-s"},
						Output: OutputOptions{
							Loggers: []Logger{NewInMemoryLogger(1)},
						},
					}
					expectedOutput := "foobar"
					stdin := []byte("echo " + expectedOutput)
					subTestCase(ctx, t, opts, expectedOutput, stdin)
				})
			}
		},
		"InvalidFilterReturnsError": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			procs, err := client.List(ctx, Filter("foo"))
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
			proc, err := client.CreateProcess(ctx, sleepCreateOpts(10))
			require.NoError(t, err)

			assert.Error(t, proc.Signal(ctx, syscall.Signal(-1)))
		},
		"GetProcessWhenInvalid": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			srv.manager = &MockManager{FailGet: true}

			_, err := client.Get(ctx, "foo")
			assert.Error(t, err)
		},
		"MetricsErrorForInvalidProcess": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			req, err := http.NewRequest(http.MethodGet, client.getURL("/process/%s/metrics", "foo"), nil)
			require.NoError(t, err)
			req = req.WithContext(ctx)
			res, err := httpClient.Do(req)
			require.NoError(t, err)

			assert.Equal(t, http.StatusNotFound, res.StatusCode)
		},
		"MetricsPopulatedForValidProcess": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			id := "foo"
			srv.manager = &MockManager{
				Procs: []Process{
					&MockProcess{ProcInfo: ProcessInfo{ID: id, PID: os.Getpid()}},
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
			srv.manager = &MockManager{
				Procs: []Process{
					&MockProcess{ProcInfo: ProcessInfo{ID: id}},
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
			srv.manager = &MockManager{
				Procs: []Process{
					&MockProcess{ProcInfo: ProcessInfo{ID: id}},
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
			file, err := ioutil.TempFile("build", "out.txt")
			require.NoError(t, err)
			defer os.Remove(file.Name())
			absPath, err := filepath.Abs(file.Name())
			require.NoError(t, err)

			assert.NoError(t, client.DownloadFile(ctx, DownloadInfo{URL: "https://example.com", Path: absPath}))

			info, err := os.Stat(file.Name())
			assert.NoError(t, err)
			assert.NotEqual(t, 0, info.Size())
		},
		"DownloadFileCreatesResourceAndExtracts": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			downloadDir, err := ioutil.TempDir("build", "rest_test")
			require.NoError(t, err)
			defer os.RemoveAll(downloadDir)

			fileServerDir, err := ioutil.TempDir("build", "rest_test_server")
			require.NoError(t, err)
			defer os.RemoveAll(fileServerDir)

			fileName := "foo.zip"
			fileContents := "foo"
			require.NoError(t, addFileToDirectory(fileServerDir, fileName, fileContents))

			absDownloadDir, err := filepath.Abs(downloadDir)
			require.NoError(t, err)
			destFilePath := filepath.Join(absDownloadDir, fileName)
			destExtractDir := filepath.Join(absDownloadDir, "extracted")

			port := getPortNumber()
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
			require.NoError(t, waitForRESTService(ctx, baseURL))

			info := DownloadInfo{
				URL:  fmt.Sprintf("%s/%s", baseURL, fileName),
				Path: destFilePath,
				ArchiveOpts: ArchiveOptions{
					ShouldExtract: true,
					Format:        ArchiveZip,
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
			fileName := filepath.Join("build", "out.txt")
			_, err := os.Stat(fileName)
			require.True(t, os.IsNotExist(err))

			info := DownloadInfo{
				URL:  "https://example.com",
				Path: fileName,
				ArchiveOpts: ArchiveOptions{
					ShouldExtract: true,
					Format:        ArchiveFormat("foo"),
				},
			}
			assert.Error(t, client.DownloadFile(ctx, info))

			_, err = os.Stat(fileName)
			assert.True(t, os.IsNotExist(err))
		},
		"DownloadFileFailsForUnarchivedFile": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			file, err := ioutil.TempFile("build", "out.txt")
			require.NoError(t, err)
			defer os.Remove(file.Name())
			extractDir, err := ioutil.TempDir("build", "out")
			require.NoError(t, err)
			defer os.RemoveAll(extractDir)

			info := DownloadInfo{
				URL:  "https://example.com",
				Path: file.Name(),
				ArchiveOpts: ArchiveOptions{
					ShouldExtract: true,
					Format:        ArchiveAuto,
				},
			}
			assert.Error(t, client.DownloadFile(ctx, info))

			dirContents, err := ioutil.ReadDir(extractDir)
			require.NoError(t, err)
			assert.Zero(t, len(dirContents))
		},
		"DownloadFileFailsWithInvalidURL": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			err := client.DownloadFile(ctx, DownloadInfo{URL: "", Path: ""})
			assert.Error(t, err)
		},
		"DownloadFileFailsForNonexistentURL": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			file, err := ioutil.TempFile("build", "out.txt")
			require.NoError(t, err)
			defer os.Remove(file.Name())
			assert.Error(t, client.DownloadFile(ctx, DownloadInfo{URL: "https://example.com/foo", Path: file.Name()}))
		},
		"DownloadFileFailsForInsufficientPermissions": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			if os.Geteuid() == 0 {
				t.Skip("cannot test download permissions as root")
			} else if runtime.GOOS == "windows" {
				t.Skip("cannot test download permissions on windows")
			}
			assert.Error(t, client.DownloadFile(ctx, DownloadInfo{URL: "https://example.com", Path: "/foo/bar"}))
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
			fileName := filepath.Join("build", "out.txt")
			absPath, err := filepath.Abs(fileName)
			require.NoError(t, err)

			body, err := makeBody(DownloadInfo{
				URL:  "://example.com",
				Path: absPath,
			})
			require.NoError(t, err)

			req, err := http.NewRequest(http.MethodPost, client.getURL("/download"), body)
			require.NoError(t, err)
			rw := httptest.NewRecorder()

			srv.downloadFile(rw, req)
			assert.Equal(t, http.StatusBadRequest, rw.Code)
		},
		"WithInMemoryLogger": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			output := "foo"
			opts := &CreateOptions{
				Args: []string{"echo", output},
				Output: OutputOptions{
					Loggers: []Logger{
						Logger{
							Type:    LogInMemory,
							Options: LogOptions{InMemoryCap: 100, Format: LogFormatPlain},
						},
					},
				},
			}

			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, proc Process){
				"GetLogStreamFailsForInvalidCount": func(ctx context.Context, t *testing.T, proc Process) {
					stream, err := client.GetLogStream(ctx, proc.ID(), -1)
					assert.Error(t, err)
					assert.Zero(t, stream)
				},
				"GetLogStreamReturnsOutputOnSuccess": func(ctx context.Context, t *testing.T, proc Process) {
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
			opts := &CreateOptions{Args: []string{"echo", "foo"}}

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
			assert.Equal(t, DefaultMaxCacheSize, srv.cacheOpts.MaxSize)
			assert.Equal(t, DefaultCachePruneDelay, srv.cacheOpts.PruneDelay)
			assert.Equal(t, false, srv.cacheOpts.Disabled)
		},
		"ConfigureCacheFailsWithInvalidOptions": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			opts := CacheOptions{PruneDelay: -1}
			assert.Error(t, client.ConfigureCache(ctx, opts))
			assert.Equal(t, DefaultMaxCacheSize, srv.cacheOpts.MaxSize)
			assert.Equal(t, DefaultCachePruneDelay, srv.cacheOpts.PruneDelay)

			opts = CacheOptions{MaxSize: -1}
			assert.Error(t, client.ConfigureCache(ctx, opts))
			assert.Equal(t, DefaultMaxCacheSize, srv.cacheOpts.MaxSize)
			assert.Equal(t, DefaultCachePruneDelay, srv.cacheOpts.PruneDelay)
		},
		"ConfigureCachePassesWithZeroOptions": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			opts := CacheOptions{}
			assert.NoError(t, client.ConfigureCache(ctx, opts))
		},
		"ConfigureCachePassesWithValidOptions": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			opts := CacheOptions{PruneDelay: 5 * time.Second, MaxSize: 1024}
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
			err := client.DownloadMongoDB(ctx, MongoDBDownloadOptions{})
			assert.Error(t, err)
		},
		"DownloadMongoDBPassesWithValidOptions": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			dir, err := ioutil.TempDir("build", "mongodb")
			require.NoError(t, err)
			defer os.RemoveAll(dir)
			absDir, err := filepath.Abs(dir)
			require.NoError(t, err)

			opts := validMongoDBDownloadOptions()
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
			opts := &CreateOptions{Args: []string{"echo", "foo"}}
			opts.Output.Loggers = []Logger{Logger{Type: LogDefault, Options: LogOptions{Format: LogFormatPlain}}}

			proc, err := client.CreateProcess(ctx, opts)
			assert.NoError(t, err)
			assert.NotNil(t, proc)

			urls, err := client.GetBuildloggerURLs(ctx, proc.ID())
			assert.Error(t, err)
			assert.Nil(t, urls)
		},
		"CreateWithMultipleLoggers": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			file, err := ioutil.TempFile("build", "out.txt")
			require.NoError(t, err)
			defer os.Remove(file.Name())

			fileLogger := Logger{
				Type: LogFile,
				Options: LogOptions{
					FileName: file.Name(),
					Format:   LogFormatPlain,
				},
			}

			inMemoryLogger := Logger{
				Type: LogInMemory,
				Options: LogOptions{
					Format:      LogFormatPlain,
					InMemoryCap: 100,
				},
			}

			opts := &CreateOptions{Output: OutputOptions{Loggers: []Logger{inMemoryLogger, fileLogger}}}
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
		"ServiceRegisterSignalTriggerIDChecksForExistingProcess": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			req, err := http.NewRequest(http.MethodPatch, client.getURL("/process/%s/trigger/signal/%s", "foo", CleanTerminationSignalTrigger), nil)
			require.NoError(t, err)

			resp, err := client.client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
			assert.Contains(t, handleError(resp).Error(), "no process 'foo' found")
		},
		"ServiceRegisterSignalTriggerIDChecksForInvalidTriggerID": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			opts := yesCreateOpts(0)
			proc, err := client.CreateProcess(ctx, &opts)
			require.NoError(t, err)
			assert.True(t, proc.Running(ctx))

			assert.Error(t, proc.RegisterSignalTriggerID(ctx, SignalTriggerID("foo")))

			assert.NoError(t, proc.Signal(ctx, syscall.SIGTERM))
		},
		"ServiceRegisterSignalTriggerIDPassesWithValidArgs": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {
			opts := yesCreateOpts(0)
			proc, err := client.CreateProcess(ctx, &opts)
			require.NoError(t, err)
			assert.True(t, proc.Running(ctx))

			assert.NoError(t, proc.RegisterSignalTriggerID(ctx, CleanTerminationSignalTrigger))

			assert.NoError(t, proc.Signal(ctx, syscall.SIGTERM))
		},
		// "": func(ctx context.Context, t *testing.T, srv *Service, client *restClient) {},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), longTaskTimeout)
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
