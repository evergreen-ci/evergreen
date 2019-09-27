package cli

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/mock"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestCLIManager(t *testing.T) {
	for remoteType, makeService := range map[string]func(ctx context.Context, t *testing.T, port int, manager jasper.Manager) jasper.CloseFunc{
		RESTService: makeTestRESTService,
		RPCService:  makeTestRPCService,
	} {
		t.Run(remoteType, func(t *testing.T) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string){
				"IDReturnsNonempty": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					resp := &IDResponse{}
					require.NoError(t, execCLICommandOutput(t, c, managerID(), resp))
					require.True(t, resp.Successful())
					assert.NotEmpty(t, resp.ID)
				},
				"CommandsWithInputFailWithInvalidInput": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(mock.Process{})
					require.NoError(t, err)
					assert.Error(t, execCLICommandInputOutput(t, c, managerCreateProcess(), input, &InfoResponse{}))
					assert.Error(t, execCLICommandInputOutput(t, c, managerCreateCommand(), input, &OutcomeResponse{}))
					assert.Error(t, execCLICommandInputOutput(t, c, managerGet(), input, &InfoResponse{}))
					assert.Error(t, execCLICommandInputOutput(t, c, managerList(), input, &InfosResponse{}))
					assert.Error(t, execCLICommandInputOutput(t, c, managerGroup(), input, &InfosResponse{}))
				},
				"CommandsWithoutInputPassWithInvalidInput": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(mock.Process{})
					require.NoError(t, err)
					resp := &OutcomeResponse{}
					assert.NoError(t, execCLICommandInputOutput(t, c, managerClear(), input, resp))
					assert.NoError(t, execCLICommandInputOutput(t, c, managerClose(), input, resp))
				},
				"CreateProcessPasses": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(options.Create{
						Args: []string{"echo", "hello", "world"},
					})
					require.NoError(t, err)
					resp := &InfoResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, managerCreateProcess(), input, resp))
					require.True(t, resp.Successful())
				},
				"CreateCommandPasses": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(options.Command{
						Commands: [][]string{[]string{"true"}},
					})
					require.NoError(t, err)
					resp := &OutcomeResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, managerCreateCommand(), input, resp))
					require.True(t, resp.Successful())
				},
				"GetExistingIDPasses": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(IDInput{jasperProcID})
					require.NoError(t, err)
					resp := &InfoResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, managerGet(), input, resp))
					require.True(t, resp.Successful())
					assert.Equal(t, jasperProcID, resp.Info.ID)
				},
				"GetNonexistentIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(IDInput{nonexistentID})
					require.NoError(t, err)
					resp := &InfoResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, managerGet(), input, resp))
					require.False(t, resp.Successful())
					require.NotEmpty(t, resp.ErrorMessage())
				},
				"GetEmptyIDFails": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(IDInput{""})
					require.NoError(t, err)
					assert.Error(t, execCLICommandInputOutput(t, c, managerGet(), input, &InfoResponse{}))
				},
				"ListValidFilterPasses": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(FilterInput{options.All})
					require.NoError(t, err)
					resp := &InfosResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, managerList(), input, resp))
					require.True(t, resp.Successful())
					assert.Len(t, resp.Infos, 1)
					assert.Equal(t, jasperProcID, resp.Infos[0].ID)
				},
				"ListInvalidFilterFails": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(FilterInput{options.Filter("foo")})
					require.NoError(t, err)
					assert.Error(t, execCLICommandInputOutput(t, c, managerList(), input, &InfosResponse{}))
				},
				"GroupFindsTaggedProcess": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					tag := "foo"
					require.True(t, tagProcess(t, c, jasperProcID, tag).Successful())

					input, err := json.Marshal(TagInput{Tag: tag})
					require.NoError(t, err)
					resp := &InfosResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, managerGroup(), input, resp))
					require.True(t, resp.Successful())
					require.Len(t, resp.Infos, 1)
					assert.Equal(t, jasperProcID, resp.Infos[0].ID)
				},
				"GroupEmptyTagFails": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(TagInput{Tag: ""})
					require.NoError(t, err)
					assert.Error(t, execCLICommandInputOutput(t, c, managerGroup(), input, &InfosResponse{}))
				},
				"GroupNoMatchingTaggedProcessesReturnsEmpty": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					input, err := json.Marshal(TagInput{Tag: "foo"})
					require.NoError(t, err)
					resp := &InfosResponse{}
					require.NoError(t, execCLICommandInputOutput(t, c, managerGroup(), input, resp))
					require.True(t, resp.Successful())
					assert.Len(t, resp.Infos, 0)
				},
				"ClearPasses": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					resp := &OutcomeResponse{}
					require.NoError(t, execCLICommandOutput(t, c, managerClear(), resp))
					assert.True(t, resp.Successful())
				},
				"ClosePasses": func(ctx context.Context, t *testing.T, c *cli.Context, jasperProcID string) {
					resp := &OutcomeResponse{}
					require.NoError(t, execCLICommandOutput(t, c, managerClose(), resp))
					assert.True(t, resp.Successful())
				},
			} {
				t.Run(testName, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), testutil.TestTimeout)
					defer cancel()
					port := testutil.GetPortNumber()
					c := mockCLIContext(remoteType, port)
					manager, err := jasper.NewLocalManager(false)
					require.NoError(t, err)
					closeService := makeService(ctx, t, port, manager)
					require.NoError(t, err)
					defer func() {
						assert.NoError(t, closeService())
					}()

					resp := &InfoResponse{}
					input, err := json.Marshal(testutil.TrueCreateOpts())
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
