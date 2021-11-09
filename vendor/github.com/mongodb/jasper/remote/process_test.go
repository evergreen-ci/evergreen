package remote

import (
	"context"
	"syscall"
	"testing"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	testoptions "github.com/mongodb/jasper/testutil/options"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessImplementations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	httpClient := utility.GetHTTPClient()
	defer utility.PutHTTPClient(httpClient)

	testCases := append(jasper.ProcessTests(), []jasper.ProcessTestCase{
		{
			Name: "RegisterTriggerFails",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc jasper.ProcessConstructor) {
				opts.Args = testoptions.SleepCreateOpts(3).Args
				proc, err := makeProc(ctx, opts)
				require.NoError(t, err)
				assert.Error(t, proc.RegisterTrigger(ctx, func(jasper.ProcessInfo) {}))
			},
		},
		{
			Name: "RegisterSignalTriggerFails",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc jasper.ProcessConstructor) {
				opts.Args = testoptions.SleepCreateOpts(3).Args
				proc, err := makeProc(ctx, opts)
				require.NoError(t, err)
				assert.Error(t, proc.RegisterSignalTrigger(ctx, func(jasper.ProcessInfo, syscall.Signal) bool {
					return false
				}))
			},
		},
	}...)

	for procName, makeProc := range map[string]jasper.ProcessConstructor{
		"REST": func(ctx context.Context, opts *options.Create) (jasper.Process, error) {
			mngr, err := jasper.NewSynchronizedManager(false)
			if err != nil {
				return nil, errors.WithStack(err)
			}

			_, client, err := makeRESTServiceAndClient(ctx, mngr, httpClient)
			if err != nil {
				return nil, errors.WithStack(err)
			}

			return client.CreateProcess(ctx, opts)
		},
		"MDB": func(ctx context.Context, opts *options.Create) (jasper.Process, error) {
			mngr, err := jasper.NewSynchronizedManager(false)
			if err != nil {
				return nil, errors.WithStack(err)
			}

			client, err := makeTestMDBServiceAndClient(ctx, mngr)
			if err != nil {
				return nil, errors.WithStack(err)
			}

			return client.CreateProcess(ctx, opts)
		},
		"RPC/TLS": func(ctx context.Context, opts *options.Create) (jasper.Process, error) {
			mngr, err := jasper.NewSynchronizedManager(false)
			if err != nil {
				return nil, errors.WithStack(err)
			}

			client, err := makeTLSRPCServiceAndClient(ctx, mngr)
			if err != nil {
				return nil, errors.WithStack(err)
			}

			return client.CreateProcess(ctx, opts)
		},
		"RPC/Insecure": func(ctx context.Context, opts *options.Create) (jasper.Process, error) {
			mngr, err := jasper.NewSynchronizedManager(false)
			if err != nil {
				return nil, errors.WithStack(err)
			}

			client, err := makeInsecureRPCServiceAndClient(ctx, mngr)
			if err != nil {
				return nil, errors.WithStack(err)
			}

			return client.CreateProcess(ctx, opts)
		},
	} {
		t.Run(procName, func(t *testing.T) {
			for optsTestName, modifyOpts := range map[string]testoptions.ModifyOpts{
				"BlockingProcess": func(opts *options.Create) *options.Create {
					opts.Implementation = options.ProcessImplementationBlocking
					return opts
				},
				"BasicProcess": func(opts *options.Create) *options.Create {
					opts.Implementation = options.ProcessImplementationBasic
					return opts
				},
			} {
				t.Run(optsTestName, func(t *testing.T) {
					for _, testCase := range testCases {
						t.Run(testCase.Name, func(t *testing.T) {
							tctx, tcancel := context.WithTimeout(ctx, testutil.RPCTestTimeout)
							defer tcancel()

							opts := modifyOpts(&options.Create{Args: []string{"ls"}})
							testCase.Case(tctx, t, opts, makeProc)
						})
					}
				})
			}
		})
	}
}
