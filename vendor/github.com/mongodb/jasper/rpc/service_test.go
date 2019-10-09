package rpc

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"

	empty "github.com/golang/protobuf/ptypes/empty"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/rpc/internal"
	"github.com/mongodb/jasper/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestRPCService(t *testing.T) {
	for managerName, makeManager := range map[string]func(trackProcs bool) (jasper.Manager, error){
		"Basic":    jasper.NewLocalManager,
		"Blocking": jasper.NewLocalManagerBlockingProcesses,
	} {
		t.Run(managerName, func(t *testing.T) {
			for testName, testCase := range map[string]func(context.Context, *testing.T, internal.JasperProcessManagerClient){
				"CreateWithLogFile": func(ctx context.Context, t *testing.T, client internal.JasperProcessManagerClient) {
					file, err := ioutil.TempFile(buildDir(t), "out.txt")
					require.NoError(t, err)
					require.NoError(t, file.Close())
					defer func() {
						assert.NoError(t, os.RemoveAll(file.Name()))
					}()

					logger := options.Logger{
						Type: options.LogFile,
						Options: options.Log{
							FileName: file.Name(),
							Format:   options.LogFormatPlain,
						},
					}
					output := "foobar"
					opts := options.Create{
						Args: []string{"echo", output},
						Output: options.Output{
							Loggers: []options.Logger{logger},
						},
					}

					procInfo, err := client.Create(ctx, internal.ConvertCreateOptions(&opts))
					require.NoError(t, err)
					require.NotNil(t, procInfo)

					outcome, err := client.Wait(ctx, &internal.JasperProcessID{Value: procInfo.Id})
					require.NoError(t, err)
					require.True(t, outcome.Success)

					info, err := os.Stat(file.Name())
					require.NoError(t, err)
					assert.NotZero(t, info.Size())

					fileContents, err := ioutil.ReadFile(file.Name())
					require.NoError(t, err)
					assert.Contains(t, string(fileContents), output)
				},
				"DownloadFileCreatesResource": func(ctx context.Context, t *testing.T, client internal.JasperProcessManagerClient) {
					file, err := ioutil.TempFile(buildDir(t), "out.txt")
					require.NoError(t, err)
					require.NoError(t, file.Close())
					defer func() {
						assert.NoError(t, os.RemoveAll(file.Name()))
					}()

					info := options.Download{
						URL:  "http://example.com",
						Path: file.Name(),
					}
					outcome, err := client.DownloadFile(ctx, internal.ConvertDownloadInfo(info))
					require.NoError(t, err)
					assert.True(t, outcome.Success)

					fileInfo, err := os.Stat(file.Name())
					require.NoError(t, err)
					assert.NotZero(t, fileInfo.Size())
				},
				"DownloadFileFailsForInvalidArchiveFormat": func(ctx context.Context, t *testing.T, client internal.JasperProcessManagerClient) {
					fileName := filepath.Join(buildDir(t), "out.txt")

					info := options.Download{
						URL:  "https://example.com",
						Path: fileName,
						ArchiveOpts: options.Archive{
							ShouldExtract: true,
							Format:        options.ArchiveFormat("foo"),
						},
					}
					outcome, err := client.DownloadFile(ctx, internal.ConvertDownloadInfo(info))
					assert.NoError(t, err)
					assert.False(t, outcome.Success)
				},
				"DownloadFileFailsForInvalidURL": func(ctx context.Context, t *testing.T, client internal.JasperProcessManagerClient) {
					fileName := filepath.Join(buildDir(t), "out.txt")

					info := options.Download{
						URL:  "://example.com",
						Path: fileName,
					}
					outcome, err := client.DownloadFile(ctx, internal.ConvertDownloadInfo(info))
					require.NoError(t, err)
					assert.False(t, outcome.Success)
				},
				"DownloadFileFailsForNonexistentURL": func(ctx context.Context, t *testing.T, client internal.JasperProcessManagerClient) {
					fileName := filepath.Join(buildDir(t), "out.txt")

					info := options.Download{
						URL:  "http://example.com/foo",
						Path: fileName,
					}
					outcome, err := client.DownloadFile(ctx, internal.ConvertDownloadInfo(info))
					require.NoError(t, err)
					assert.False(t, outcome.Success)
				},
				"GetBuildloggerURLsFailsWithNonexistentProcess": func(ctx context.Context, t *testing.T, client internal.JasperProcessManagerClient) {
					urls, err := client.GetBuildloggerURLs(ctx, &internal.JasperProcessID{Value: "foo"})
					assert.Error(t, err)
					assert.Nil(t, urls)
				},
				"GetBuildloggerURLsFailsWithoutBuildlogger": func(ctx context.Context, t *testing.T, client internal.JasperProcessManagerClient) {
					logger := options.Logger{
						Type:    options.LogDefault,
						Options: options.Log{Format: options.LogFormatPlain},
					}
					opts := options.Create{
						Args: []string{"echo", "foobar"},
						Output: options.Output{
							Loggers: []options.Logger{logger},
						},
					}

					info, err := client.Create(ctx, internal.ConvertCreateOptions(&opts))
					require.NoError(t, err)

					urls, err := client.GetBuildloggerURLs(ctx, &internal.JasperProcessID{Value: info.Id})
					assert.Error(t, err)
					assert.Nil(t, urls)
				},
				"RegisterSignalTriggerIDChecksForExistingProcess": func(ctx context.Context, t *testing.T, client internal.JasperProcessManagerClient) {
					outcome, err := client.RegisterSignalTriggerID(ctx, internal.ConvertSignalTriggerParams("foo", jasper.CleanTerminationSignalTrigger))
					require.NoError(t, err)
					assert.False(t, outcome.Success)
				},
				"RegisterSignalTriggerIDFailsForInvalidTriggerID": func(ctx context.Context, t *testing.T, client internal.JasperProcessManagerClient) {
					opts := testutil.SleepCreateOpts(10)
					info, err := client.Create(ctx, internal.ConvertCreateOptions(opts))
					require.NoError(t, err)

					outcome, err := client.RegisterSignalTriggerID(ctx, internal.ConvertSignalTriggerParams(info.Id, jasper.SignalTriggerID("")))
					require.NoError(t, err)
					assert.False(t, outcome.Success)

					outcome, err = client.Close(ctx, &empty.Empty{})
					require.NoError(t, err)
					assert.True(t, outcome.Success)
				},
				"RegisterSignalTriggerIDPassesWithValidArgs": func(ctx context.Context, t *testing.T, client internal.JasperProcessManagerClient) {
					opts := testutil.SleepCreateOpts(10)
					info, err := client.Create(ctx, internal.ConvertCreateOptions(opts))
					require.NoError(t, err)

					outcome, err := client.RegisterSignalTriggerID(ctx, internal.ConvertSignalTriggerParams(info.Id, jasper.CleanTerminationSignalTrigger))
					require.NoError(t, err)
					assert.True(t, outcome.Success)

					outcome, err = client.Close(ctx, &empty.Empty{})
					require.NoError(t, err)
					assert.True(t, outcome.Success)
				},
				//"": func(ctx context.Context, t *testing.T, client internal.JasperProcessManagerClient) {},
			} {
				t.Run(testName, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), testutil.TestTimeout)
					defer cancel()

					manager, err := makeManager(false)
					require.NoError(t, err)
					addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("localhost:%d", testutil.GetPortNumber()))
					require.NoError(t, err)
					require.NoError(t, startTestService(ctx, manager, addr, nil))

					conn, err := grpc.DialContext(ctx, addr.String(), grpc.WithInsecure(), grpc.WithBlock())
					require.NoError(t, err)
					client := internal.NewJasperProcessManagerClient(conn)

					go func() {
						<-ctx.Done()
						conn.Close()
					}()

					testCase(ctx, t, client)
				})
			}
		})
	}
}
