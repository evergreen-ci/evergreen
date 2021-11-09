package remote

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	testoptions "github.com/mongodb/jasper/testutil/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	sender := grip.GetSender()
	grip.Error(sender.SetLevel(send.LevelInfo{Default: level.Info, Threshold: level.Info}))
	grip.Error(grip.SetSender(sender))
}

func remoteManagerTestCases(httpClient *http.Client) map[string]func(context.Context, *testing.T) Manager {
	return map[string]func(context.Context, *testing.T) Manager{
		"MDB": func(ctx context.Context, t *testing.T) Manager {
			mngr, err := jasper.NewSynchronizedManager(false)
			require.NoError(t, err)

			client, err := makeTestMDBServiceAndClient(ctx, mngr)
			require.NoError(t, err)
			return client
		},
		"RPC/TLS": func(ctx context.Context, t *testing.T) Manager {
			mngr, err := jasper.NewSynchronizedManager(false)
			require.NoError(t, err)

			client, err := makeTLSRPCServiceAndClient(ctx, mngr)
			require.NoError(t, err)
			return client
		},
		"RPC/Insecure": func(ctx context.Context, t *testing.T) Manager {
			assert.NotPanics(t, func() {
				newRPCClient(nil)
			})

			mngr, err := jasper.NewSynchronizedManager(false)
			require.NoError(t, err)

			client, err := makeInsecureRPCServiceAndClient(ctx, mngr)
			require.NoError(t, err)
			return client
		},
		"REST": func(ctx context.Context, t *testing.T) Manager {
			mngr, err := jasper.NewSynchronizedManager(false)
			require.NoError(t, err)

			_, client, err := makeRESTServiceAndClient(ctx, mngr, httpClient)
			require.NoError(t, err)
			return client
		},
	}
}

func TestManagerImplementations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	httpClient := utility.GetHTTPClient()
	defer utility.PutHTTPClient(httpClient)

	testCases := append(jasper.ManagerTests(), []jasper.ManagerTestCase{
		{
			Name: "WaitingOnNonexistentProcessErrors",
			Case: func(ctx context.Context, t *testing.T, mngr jasper.Manager, modifyOpts testoptions.ModifyOpts) {
				opts := modifyOpts(testoptions.TrueCreateOpts())

				proc, err := mngr.CreateProcess(ctx, opts)
				require.NoError(t, err)

				_, err = proc.Wait(ctx)
				require.NoError(t, err)

				mngr.Clear(ctx)

				_, err = proc.Wait(ctx)
				require.Error(t, err)
				procs, err := mngr.List(ctx, options.All)
				require.NoError(t, err)
				assert.Len(t, procs, 0)
			},
		},
		{
			Name: "RegisterProcessAlwaysErrors",
			Case: func(ctx context.Context, t *testing.T, mngr jasper.Manager, modifyOpts testoptions.ModifyOpts) {
				proc, err := mngr.CreateProcess(ctx, &options.Create{Args: []string{"ls"}})
				assert.NotNil(t, proc)
				require.NoError(t, err)

				assert.Error(t, mngr.Register(ctx, nil))
				assert.Error(t, mngr.Register(ctx, proc))
			},
		},
	}...)

	for managerName, makeManager := range remoteManagerTestCases(httpClient) {
		t.Run(managerName, func(t *testing.T) {
			for _, testCase := range testCases {
				t.Run(testCase.Name, func(t *testing.T) {
					for optsTestCase, modifyOpts := range map[string]testoptions.ModifyOpts{
						"BlockingProcess": func(opts *options.Create) *options.Create {
							opts.Implementation = options.ProcessImplementationBlocking
							return opts
						},
						"BasicProcess": func(opts *options.Create) *options.Create {
							opts.Implementation = options.ProcessImplementationBasic
							return opts
						},
					} {
						t.Run(optsTestCase, func(t *testing.T) {
							tctx, tcancel := context.WithTimeout(ctx, testutil.RPCTestTimeout)
							defer tcancel()
							mngr := makeManager(tctx, t)
							testCase.Case(tctx, t, mngr, modifyOpts)
						})
					}
				})
			}
		})
	}
}

type clientTestCase struct {
	Name string
	Case func(context.Context, *testing.T, Manager)
}

func TestClientImplementations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	httpClient := utility.GetHTTPClient()
	defer utility.PutHTTPClient(httpClient)

	for managerName, makeManager := range remoteManagerTestCases(httpClient) {
		t.Run(managerName, func(t *testing.T) {
			for _, testCase := range []clientTestCase{
				{
					Name: "StandardInput",
					Case: func(ctx context.Context, t *testing.T, mngr Manager) {
						for subTestName, subTestCase := range map[string]func(ctx context.Context, t *testing.T, opts *options.Create, expectedOutput string, stdin []byte){
							"ReaderIsIgnored": func(ctx context.Context, t *testing.T, opts *options.Create, expectedOutput string, stdin []byte) {
								opts.StandardInput = bytes.NewBuffer(stdin)

								proc, err := mngr.CreateProcess(ctx, opts)
								require.NoError(t, err)

								_, err = proc.Wait(ctx)
								require.NoError(t, err)

								logs, err := mngr.GetLogStream(ctx, proc.ID(), 1)
								require.NoError(t, err)
								assert.Empty(t, logs.Logs)
							},
							"BytesSetsProcessStandardInput": func(ctx context.Context, t *testing.T, opts *options.Create, expectedOutput string, stdin []byte) {
								opts.StandardInputBytes = stdin

								proc, err := mngr.CreateProcess(ctx, opts)
								require.NoError(t, err)

								_, err = proc.Wait(ctx)
								require.NoError(t, err)

								logs, err := mngr.GetLogStream(ctx, proc.ID(), 1)
								require.NoError(t, err)

								require.Len(t, logs.Logs, 1)
								assert.Equal(t, expectedOutput, strings.TrimSpace(logs.Logs[0]))
							},
							"BytesCopiedByRespawnedProcess": func(ctx context.Context, t *testing.T, opts *options.Create, expectedOutput string, stdin []byte) {
								opts.StandardInputBytes = stdin

								proc, err := mngr.CreateProcess(ctx, opts)
								require.NoError(t, err)

								_, err = proc.Wait(ctx)
								require.NoError(t, err)

								logs, err := mngr.GetLogStream(ctx, proc.ID(), 1)
								require.NoError(t, err)

								require.Len(t, logs.Logs, 1)
								assert.Equal(t, expectedOutput, strings.TrimSpace(logs.Logs[0]))

								newProc, err := proc.Respawn(ctx)
								require.NoError(t, err)

								_, err = newProc.Wait(ctx)
								require.NoError(t, err)

								logs, err = mngr.GetLogStream(ctx, newProc.ID(), 1)
								require.NoError(t, err)

								require.Len(t, logs.Logs, 1)
								assert.Equal(t, expectedOutput, strings.TrimSpace(logs.Logs[0]))
							},
						} {
							t.Run(subTestName, func(t *testing.T) {
								inMemLogger, err := jasper.NewInMemoryLogger(1)
								require.NoError(t, err)

								opts := &options.Create{
									Args: []string{"bash", "-s"},
									Output: options.Output{
										Loggers: []*options.LoggerConfig{inMemLogger},
									},
								}

								expectedOutput := "foobar"
								stdin := []byte("echo " + expectedOutput)
								subTestCase(ctx, t, opts, expectedOutput, stdin)
							})
						}
					},
				},
				{
					Name: "GetLogStreamFromNonexistentProcessFails",
					Case: func(ctx context.Context, t *testing.T, mngr Manager) {
						stream, err := mngr.GetLogStream(ctx, "foo", 1)
						assert.Error(t, err)
						assert.Zero(t, stream)
					},
				},
				{
					Name: "GetLogStreamFailsWithoutInMemoryLogger",
					Case: func(ctx context.Context, t *testing.T, mngr Manager) {
						opts := &options.Create{Args: []string{"echo", "foo"}}
						proc, err := mngr.CreateProcess(ctx, opts)
						require.NoError(t, err)
						require.NotNil(t, proc)

						_, err = proc.Wait(ctx)
						require.NoError(t, err)

						stream, err := mngr.GetLogStream(ctx, proc.ID(), 1)
						assert.Error(t, err)
						assert.Zero(t, stream)
					},
				},
				{
					Name: "WithInMemoryLogger",
					Case: func(ctx context.Context, t *testing.T, mngr Manager) {
						inMemLogger, err := jasper.NewInMemoryLogger(100)
						require.NoError(t, err)
						output := "foo"
						opts := &options.Create{
							Args: []string{"echo", output},
							Output: options.Output{
								Loggers: []*options.LoggerConfig{inMemLogger},
							},
						}

						for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, proc jasper.Process){
							"GetLogStreamFailsForInvalidCount": func(ctx context.Context, t *testing.T, proc jasper.Process) {
								stream, err := mngr.GetLogStream(ctx, proc.ID(), -1)
								assert.Error(t, err)
								assert.Zero(t, stream)
							},
							"GetLogStreamReturnsOutputOnSuccess": func(ctx context.Context, t *testing.T, proc jasper.Process) {
								logs := []string{}
								for stream, err := mngr.GetLogStream(ctx, proc.ID(), 1); !stream.Done; stream, err = mngr.GetLogStream(ctx, proc.ID(), 1) {
									require.NoError(t, err)
									require.NotEmpty(t, stream.Logs)
									logs = append(logs, stream.Logs...)
								}
								assert.Contains(t, logs, output)
							},
						} {
							t.Run(testName, func(t *testing.T) {
								proc, err := mngr.CreateProcess(ctx, opts)
								require.NoError(t, err)
								require.NotNil(t, proc)

								_, err = proc.Wait(ctx)
								require.NoError(t, err)
								testCase(ctx, t, proc)
							})
						}
					},
				},
				{
					Name: "DownloadFile",
					Case: func(ctx context.Context, t *testing.T, mngr Manager) {
						for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, mngr Manager, tempDir string){
							"CreatesFileIfNonexistent": func(ctx context.Context, t *testing.T, mngr Manager, tempDir string) {
								opts := options.Download{
									URL:  "https://example.com",
									Path: filepath.Join(tempDir, filepath.Base(t.Name())),
								}
								require.NoError(t, mngr.DownloadFile(ctx, opts))
								defer func() {
									assert.NoError(t, os.RemoveAll(opts.Path))
								}()

								fileInfo, err := os.Stat(opts.Path)
								require.NoError(t, err)
								assert.NotZero(t, fileInfo.Size())
							},
							"WritesFileIfExists": func(ctx context.Context, t *testing.T, mngr Manager, tempDir string) {
								file, err := ioutil.TempFile(tempDir, "out.txt")
								require.NoError(t, err)
								defer func() {
									assert.NoError(t, os.RemoveAll(file.Name()))
								}()
								require.NoError(t, file.Close())

								opts := options.Download{
									URL:  "https://example.com",
									Path: file.Name(),
								}
								require.NoError(t, mngr.DownloadFile(ctx, opts))
								defer func() {
									assert.NoError(t, os.RemoveAll(opts.Path))
								}()

								fileInfo, err := os.Stat(file.Name())
								require.NoError(t, err)
								assert.NotZero(t, fileInfo.Size())
							},
							"CreatesFileAndExtracts": func(ctx context.Context, t *testing.T, mngr Manager, tempDir string) {
								downloadDir, err := ioutil.TempDir(tempDir, "out")
								require.NoError(t, err)
								defer func() {
									assert.NoError(t, os.RemoveAll(downloadDir))
								}()

								fileServerDir, err := ioutil.TempDir(tempDir, "file_server")
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
									grip.Info(fileServer.Serve(listener))
								}()

								baseURL := fmt.Sprintf("http://%s", fileServerAddr)
								require.NoError(t, testutil.WaitForHTTPService(ctx, baseURL, httpClient))

								opts := options.Download{
									URL:  fmt.Sprintf("%s/%s", baseURL, fileName),
									Path: destFilePath,
									ArchiveOpts: options.Archive{
										ShouldExtract: true,
										Format:        options.ArchiveZip,
										TargetPath:    destExtractDir,
									},
								}
								require.NoError(t, mngr.DownloadFile(ctx, opts))

								fileInfo, err := os.Stat(destFilePath)
								require.NoError(t, err)
								assert.NotZero(t, fileInfo.Size())

								dirContents, err := ioutil.ReadDir(destExtractDir)
								require.NoError(t, err)

								assert.NotZero(t, len(dirContents))
							},
							"FailsForInvalidArchiveFormat": func(ctx context.Context, t *testing.T, mngr Manager, tempDir string) {
								file, err := ioutil.TempFile(tempDir, filepath.Base(t.Name()))
								require.NoError(t, err)
								defer func() {
									assert.NoError(t, os.RemoveAll(file.Name()))
								}()
								require.NoError(t, file.Close())
								extractDir, err := ioutil.TempDir(tempDir, filepath.Base(t.Name())+"_extract")
								require.NoError(t, err)
								defer func() {
									assert.NoError(t, os.RemoveAll(file.Name()))
								}()

								opts := options.Download{
									URL:  "https://example.com",
									Path: file.Name(),
									ArchiveOpts: options.Archive{
										ShouldExtract: true,
										Format:        options.ArchiveFormat("foo"),
										TargetPath:    extractDir,
									},
								}
								assert.Error(t, mngr.DownloadFile(ctx, opts))
							},
							"FailsForUnarchivedFile": func(ctx context.Context, t *testing.T, mngr Manager, tempDir string) {
								extractDir, err := ioutil.TempDir(tempDir, filepath.Base(t.Name())+"_extract")
								require.NoError(t, err)
								defer func() {
									assert.NoError(t, os.RemoveAll(extractDir))
								}()
								opts := options.Download{
									URL:  "https://example.com",
									Path: filepath.Join(tempDir, filepath.Base(t.Name())),
									ArchiveOpts: options.Archive{
										ShouldExtract: true,
										Format:        options.ArchiveAuto,
										TargetPath:    extractDir,
									},
								}
								assert.Error(t, mngr.DownloadFile(ctx, opts))

								dirContents, err := ioutil.ReadDir(extractDir)
								require.NoError(t, err)
								assert.Zero(t, len(dirContents))
							},
							"FailsForInvalidURL": func(ctx context.Context, t *testing.T, mngr Manager, tempDir string) {
								file, err := ioutil.TempFile(tempDir, filepath.Base(t.Name()))
								require.NoError(t, err)
								defer func() {
									assert.NoError(t, os.RemoveAll(file.Name()))
								}()
								require.NoError(t, file.Close())
								assert.Error(t, mngr.DownloadFile(ctx, options.Download{URL: "", Path: file.Name()}))
							},
							"FailsForNonexistentURL": func(ctx context.Context, t *testing.T, mngr Manager, tempDir string) {
								file, err := ioutil.TempFile(tempDir, "out.txt")
								require.NoError(t, err)
								defer func() {
									assert.NoError(t, os.RemoveAll(file.Name()))
								}()
								require.NoError(t, file.Close())
								assert.Error(t, mngr.DownloadFile(ctx, options.Download{URL: "https://example.com/foo", Path: file.Name()}))
							},
							"FailsForInsufficientPermissions": func(ctx context.Context, t *testing.T, mngr Manager, tempDir string) {
								if os.Geteuid() == 0 {
									t.Skip("cannot test download permissions as root")
								} else if runtime.GOOS == "windows" {
									t.Skip("cannot test download permissions on windows")
								}
								assert.Error(t, mngr.DownloadFile(ctx, options.Download{URL: "https://example.com", Path: "/foo/bar"}))
							},
						} {
							t.Run(testName, func(t *testing.T) {
								tempDir, err := ioutil.TempDir(testutil.BuildDirectory(), filepath.Base(t.Name()))
								require.NoError(t, err)
								defer func() {
									assert.NoError(t, os.RemoveAll(tempDir))
								}()
								testCase(ctx, t, mngr, tempDir)
							})
						}
					},
				},
				{
					Name: "GetBuildloggerURLsFailsWithoutBuildlogger",
					Case: func(ctx context.Context, t *testing.T, mngr Manager) {
						logger := &options.LoggerConfig{}
						require.NoError(t, logger.Set(&options.DefaultLoggerOptions{
							Base: options.BaseOptions{Format: options.LogFormatPlain},
						}))
						opts := &options.Create{
							Args: []string{"echo", "foobar"},
							Output: options.Output{
								Loggers: []*options.LoggerConfig{logger},
							},
						}

						info, err := mngr.CreateProcess(ctx, opts)
						require.NoError(t, err)
						id := info.ID()
						assert.NotEmpty(t, id)

						urls, err := mngr.GetBuildloggerURLs(ctx, id)
						assert.Error(t, err)
						assert.Nil(t, urls)
					},
				},
				{
					Name: "GetBuildloggerURLsFailsWithNonexistentProcess",
					Case: func(ctx context.Context, t *testing.T, mngr Manager) {
						urls, err := mngr.GetBuildloggerURLs(ctx, "foo")
						assert.Error(t, err)
						assert.Nil(t, urls)
					},
				},
				{
					Name: "CreateProcessWithLogFile",
					Case: func(ctx context.Context, t *testing.T, mngr Manager) {
						file, err := ioutil.TempFile(testutil.BuildDirectory(), filepath.Base(t.Name()))
						require.NoError(t, err)
						defer func() {
							assert.NoError(t, os.RemoveAll(file.Name()))
						}()
						require.NoError(t, file.Close())

						logger := &options.LoggerConfig{}
						require.NoError(t, logger.Set(&options.FileLoggerOptions{
							Filename: file.Name(),
							Base:     options.BaseOptions{Format: options.LogFormatPlain},
						}))
						output := "foobar"
						opts := &options.Create{
							Args: []string{"echo", output},
							Output: options.Output{
								Loggers: []*options.LoggerConfig{logger},
							},
						}

						proc, err := mngr.CreateProcess(ctx, opts)
						require.NoError(t, err)

						exitCode, err := proc.Wait(ctx)
						require.NoError(t, err)
						require.Zero(t, exitCode)

						info, err := os.Stat(file.Name())
						require.NoError(t, err)
						assert.NotZero(t, info.Size())

						fileContents, err := ioutil.ReadFile(file.Name())
						require.NoError(t, err)
						assert.Contains(t, string(fileContents), output)
					},
				},
				{
					Name: "RegisterSignalTriggerIDChecksForInvalidTriggerID",
					Case: func(ctx context.Context, t *testing.T, mngr Manager) {
						opts := testoptions.SleepCreateOpts(1)
						proc, err := mngr.CreateProcess(ctx, opts)
						require.NoError(t, err)
						assert.True(t, proc.Running(ctx))

						assert.Error(t, proc.RegisterSignalTriggerID(ctx, jasper.SignalTriggerID("foo")))

						assert.NoError(t, proc.Signal(ctx, syscall.SIGTERM))
					},
				},
				{
					Name: "RegisterSignalTriggerIDPassesWithValidArgs",
					Case: func(ctx context.Context, t *testing.T, mngr Manager) {
						opts := testoptions.SleepCreateOpts(1)
						proc, err := mngr.CreateProcess(ctx, opts)
						require.NoError(t, err)
						assert.True(t, proc.Running(ctx))

						assert.NoError(t, proc.RegisterSignalTriggerID(ctx, jasper.CleanTerminationSignalTrigger))

						assert.NoError(t, proc.Signal(ctx, syscall.SIGTERM))
					},
				},
				{
					Name: "SendMessagesFailsWithNonexistentLogger",
					Case: func(ctx context.Context, t *testing.T, mngr Manager) {
						payload := options.LoggingPayload{
							LoggerID: "nonexistent",
							Data:     "new log message",
							Priority: level.Warning,
							Format:   options.LoggingPayloadFormatString,
						}
						assert.Error(t, mngr.SendMessages(ctx, payload))
					},
				},
				{
					Name: "SendMessagesSucceeds",
					Case: func(ctx context.Context, t *testing.T, mngr Manager) {
						lc := mngr.LoggingCache(ctx)
						tmpDir, err := ioutil.TempDir(testutil.BuildDirectory(), "logging_cache")
						require.NoError(t, err)
						defer func() {
							assert.NoError(t, os.RemoveAll(tmpDir))
						}()
						tmpFile := filepath.Join(tmpDir, "send_messages")

						fileOpts := &options.FileLoggerOptions{
							Filename: tmpFile,
							Base: options.BaseOptions{
								Format: options.LogFormatPlain,
							},
						}
						config := &options.LoggerConfig{}
						require.NoError(t, config.Set(fileOpts))

						logger, err := lc.Create("logger", &options.Output{
							Loggers: []*options.LoggerConfig{config},
						})
						require.NoError(t, err)
						defer func() {
							assert.NoError(t, lc.Clear(ctx))
						}()

						payload := options.LoggingPayload{
							LoggerID: logger.ID,
							Data:     "new log message",
							Priority: level.Info,
							Format:   options.LoggingPayloadFormatString,
						}
						assert.NoError(t, mngr.SendMessages(ctx, payload))

						content, err := ioutil.ReadFile(tmpFile)
						require.NoError(t, err)
						assert.Equal(t, payload.Data, strings.TrimSpace(string(content)))
					},
				},
				{
					Name: "CreateScriptingSucceeds",
					Case: func(ctx context.Context, t *testing.T, mngr Manager) {
						tmpDir, err := ioutil.TempDir(testutil.BuildDirectory(), "scripting_tests")
						require.NoError(t, err)
						defer func() {
							assert.NoError(t, os.RemoveAll(tmpDir))
						}()
						sh := createTestScriptingHarness(ctx, t, mngr, tmpDir)
						assert.NotZero(t, sh)
					},
				},
				{
					Name: "CreateScriptingFailsWithInvalidOptions",
					Case: func(ctx context.Context, t *testing.T, mngr Manager) {
						sh, err := mngr.CreateScripting(ctx, &options.ScriptingGolang{})
						assert.Error(t, err)
						assert.Zero(t, sh)
					},
				},
				{
					Name: "GetScriptingWithNonexistentHarnessFails",
					Case: func(ctx context.Context, t *testing.T, mngr Manager) {
						_, err := mngr.GetScripting(ctx, "nonexistent")
						assert.Error(t, err)
					},
				},
				{
					Name: "GetScriptingWithExistingHarnessSucceeds",
					Case: func(ctx context.Context, t *testing.T, mngr Manager) {
						tmpDir, err := ioutil.TempDir(testutil.BuildDirectory(), "scripting_tests")
						require.NoError(t, err)
						defer func() {
							assert.NoError(t, os.RemoveAll(tmpDir))
						}()
						expectedHarness := createTestScriptingHarness(ctx, t, mngr, tmpDir)

						harness, err := mngr.GetScripting(ctx, expectedHarness.ID())
						require.NoError(t, err)
						assert.Equal(t, expectedHarness.ID(), harness.ID())
					},
				},
			} {
				t.Run(testCase.Name, func(t *testing.T) {
					tctx, tcancel := context.WithTimeout(ctx, testutil.RPCTestTimeout)
					defer tcancel()
					mngr := makeManager(tctx, t)
					testCase.Case(tctx, t, mngr)
				})
			}
		})
	}
}
