package cli

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func tagProcess(t *testing.T, c *cli.Context, jasperProcID string, tag string) OutcomeResponse {
	input, err := json.Marshal(TagIDInput{ID: jasperProcID, Tag: tag})
	require.NoError(t, err)
	resp := OutcomeResponse{}
	require.NoError(t, execCLICommandInputOutput(t, c, processTag(), input, &resp))
	return resp
}

const nonexistentID = "nonexistent"

func TestCLIProcess(t *testing.T) {
	for remoteType, makeService := range map[string]func(ctx context.Context, t *testing.T, port int, manager jasper.Manager) jasper.CloseFunc{
		restService: makeTestRESTService,
		rpcService:  makeTestRPCService,
	} {
		t.Run(remoteType, func(t *testing.T) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string){
				"InfoWithExistingIDSucceeds": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(IDInput{jasperProcID})
					require.NoError(t, err)
					resp := &InfoResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, processInfo(), input, resp))
					require.True(t, resp.Successful())
					assert.Equal(t, jasperProcID, resp.Info.ID)
					assert.True(t, resp.Info.IsRunning)
				},
				"InfoWithNonexistentIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(IDInput{nonexistentID})
					require.NoError(t, err)
					resp := &InfoResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, processInfo(), input, resp))
					require.False(t, resp.Successful())
				},
				"InfoWithEmptyIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(IDInput{})
					require.NoError(t, err)
					assert.Error(t, execCLICommandInputOutput(t, c, processInfo(), input, &InfoResponse{}))
				},
				"RunningWithExistingIDSucceeds": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(IDInput{jasperProcID})
					require.NoError(t, err)
					resp := &RunningResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, processRunning(), input, resp))
					require.True(t, resp.Successful())
					assert.True(t, resp.Running)
				},
				"RunningWithNonexistentIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(IDInput{nonexistentID})
					require.NoError(t, err)
					resp := &RunningResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, processRunning(), input, resp))
					assert.False(t, resp.Successful())
				},
				"RunningWithEmptyIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(IDInput{})
					require.NoError(t, err)
					assert.Error(t, execCLICommandInputOutput(t, c, processRunning(), input, &RunningResponse{}))
				},
				"CompleteWithExistingIDSucceeds": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(IDInput{jasperProcID})
					require.NoError(t, err)
					resp := &CompleteResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, processComplete(), input, resp))
					require.True(t, resp.Successful())
					assert.False(t, resp.Complete)
				},
				"CompleteWithNonexistentIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(IDInput{nonexistentID})
					require.NoError(t, err)
					resp := &CompleteResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, processComplete(), input, resp))
					assert.False(t, resp.Successful())
				},
				"CompleteWithEmptyIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(IDInput{})
					require.NoError(t, err)
					assert.Error(t, execCLICommandInputOutput(t, c, processComplete(), input, &CompleteResponse{}))
				},
				"WaitWithExistingIDSucceeds": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(IDInput{jasperProcID})
					require.NoError(t, err)
					resp := &WaitResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, processWait(), input, resp))
					require.True(t, resp.Successful())
					assert.Empty(t, resp.Error)
					assert.Zero(t, resp.ExitCode)
				},
				"WaitWithNonexistentIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(IDInput{nonexistentID})
					require.NoError(t, err)
					resp := &WaitResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, processWait(), input, resp))
					assert.False(t, resp.Successful())
				},
				"WaitWithEmptyIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(IDInput{})
					require.NoError(t, err)
					assert.Error(t, execCLICommandInputOutput(t, c, processWait(), input, &WaitResponse{}))
				},
				"Respawn": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(IDInput{jasperProcID})
					require.NoError(t, err)
					resp := &InfoResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, processRespawn(), input, resp))
					require.True(t, resp.Successful())
					assert.NotZero(t, resp.Info.ID)
					assert.NotEqual(t, resp.Info.ID, jasperProcID)
				},
				"RegisterSignalTriggerID": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(SignalTriggerIDInput{ID: jasperProcID, SignalTriggerID: jasper.CleanTerminationSignalTrigger})
					require.NoError(t, err)
					resp := &OutcomeResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, processRegisterSignalTriggerID(), input, resp))
					require.True(t, resp.Successful())
				},
				"Tag": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					require.True(t, tagProcess(t, c, jasperProcID, "foo").Successful())
				},
				"TagNonexistentIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					require.False(t, tagProcess(t, c, nonexistentID, "foo").Successful())
				},
				"TagEmptyIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					require.False(t, tagProcess(t, c, nonexistentID, "foo").Successful())
				},
				"TagEmptyTagFails": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(TagIDInput{ID: jasperProcID})
					require.NoError(t, err)
					assert.Error(t, execCLICommandInputOutput(t, c, processTag(), input, &OutcomeResponse{}))
				},
				"GetTags": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					tag := "foo"
					require.True(t, tagProcess(t, c, jasperProcID, tag).Successful())

					input, err := json.Marshal(IDInput{jasperProcID})
					require.NoError(t, err)
					resp := &TagsResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, processGetTags(), input, resp))
					require.True(t, resp.Successful())
					require.Len(t, resp.Tags, 1)
					assert.Equal(t, tag, resp.Tags[0])
				},
				"ResetTags": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					tag := "foo"
					require.True(t, tagProcess(t, c, jasperProcID, tag).Successful())

					input, err := json.Marshal(IDInput{jasperProcID})
					require.NoError(t, err)
					resp := &OutcomeResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, processResetTags(), input, resp))
					require.True(t, resp.Successful())

					getTagsInput, err := json.Marshal(IDInput{jasperProcID})
					require.NoError(t, err)
					getTagsResp := &TagsResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, processGetTags(), getTagsInput, getTagsResp))
					require.True(t, getTagsResp.Successful())
					assert.Empty(t, getTagsResp.Tags)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
					defer cancel()
					port := getNextPort()
					c := mockCLIContext(remoteType, port)
					manager, err := jasper.NewLocalManager(false)
					require.NoError(t, err)
					closeService := makeService(ctx, t, port, manager)
					require.NoError(t, err)
					defer func() {
						assert.NoError(t, closeService())
					}()

					resp := &InfoResponse{}
					input, err := json.Marshal(sleepCreateOpts(int(testTimeout.Seconds()) - 1))
					require.NoError(t, err)
					require.NoError(t, execCLICommandInputOutput(t, c, managerCreateProcess(), input, resp))
					require.True(t, resp.Successful())
					require.NotZero(t, resp.Info.ID)

					testCase(ctx, t, c, resp.Info.ID)
				})
			}
		})
	}
}
