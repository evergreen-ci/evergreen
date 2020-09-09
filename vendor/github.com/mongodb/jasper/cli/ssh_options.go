package cli

import (
	"fmt"

	"github.com/mongodb/grip"
)

// ClientOptions represents the options to connect the CLI client to the Jasper
// service.
type ClientOptions struct {
	BinaryPath          string
	Type                string
	Host                string
	Port                int
	CredentialsFilePath string
}

// Validate checks that the binary path is set and it is a recognized Jasper
// client type.
func (opts *ClientOptions) Validate() error {
	catcher := grip.NewBasicCatcher()
	if opts.BinaryPath == "" {
		catcher.New("client binary path cannot be empty")
	}
	if opts.Type != RPCService && opts.Type != RESTService {
		catcher.New("client type must be RPC or REST")
	}
	return catcher.Resolve()
}

// buildCommand returns the Jasper CLI command that will be run over SSH using
// the Jasper manager.
func (opts *ClientOptions) buildCommand(clientSubcommand ...string) []string {
	args := append(
		[]string{
			opts.BinaryPath,
			JasperCommand,
			ClientCommand,
		},
		clientSubcommand...,
	)
	args = append(args, fmt.Sprintf("--%s=%s", serviceFlagName, opts.Type))

	if opts.Host != "" {
		args = append(args, fmt.Sprintf("--%s=%s", hostFlagName, opts.Host))
	}

	if opts.Port != 0 {
		args = append(args, fmt.Sprintf("--%s=%d", portFlagName, opts.Port))
	}

	if opts.CredentialsFilePath != "" {
		args = append(args, fmt.Sprintf("--%s=%s", credsFilePathFlagName, opts.CredentialsFilePath))
	}

	return args
}
