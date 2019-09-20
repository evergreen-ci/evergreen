package cli

import (
	"fmt"

	"github.com/mongodb/grip"
	"github.com/mongodb/jasper/options"
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

// sshClientOptions represents the options necessary to run a Jasper CLI
// command over SSH.
type sshClientOptions struct {
	Machine options.Remote
	Client  ClientOptions
}

// args returns the Jasper CLI command that will be run over SSH.
func (opts *sshClientOptions) buildCommand(clientSubcommand ...string) []string {
	args := append(
		[]string{
			opts.Client.BinaryPath,
			JasperCommand,
			ClientCommand,
		},
		clientSubcommand...,
	)
	args = append(args, fmt.Sprintf("--%s=%s", serviceFlagName, opts.Client.Type))

	if opts.Client.Host != "" {
		args = append(args, fmt.Sprintf("--%s=%s", hostFlagName, opts.Client.Host))
	}

	if opts.Client.Port != 0 {
		args = append(args, fmt.Sprintf("--%s=%d", portFlagName, opts.Client.Port))
	}

	if opts.Client.CredentialsFilePath != "" {
		args = append(args, fmt.Sprintf("--%s=%s", credsFilePathFlagName, opts.Client.CredentialsFilePath))
	}

	return args
}
