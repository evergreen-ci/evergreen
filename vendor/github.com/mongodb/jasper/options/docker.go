package options

import (
	"fmt"
	"net"
	"runtime"

	"github.com/docker/docker/client"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// Docker encapsulates options related to connecting to a Docker daemon.
type Docker struct {
	Host       string `bson:"host,omitempty" json:"host,omitempty" yaml:"host,omitempty"`
	Port       int    `bson:"port,omitempty" json:"port,omitempty" yaml:"port,omitempty"`
	APIVersion string `bson:"api_version,omitempty" json:"api_version,omitempty" yaml:"api_version,omitempty"`
	Image      string `bson:"image,omitempty" json:"image,omitempty" yaml:"image,omitempty"`
	// Platform refers to the major operating system on which the Docker
	// container runs.
	Platform string `bson:"platform,omitempty" json:"platform,omitempty" yaml:"platform,omitempty"`
}

// Validate checks whether all the required fields are set and sets defaults if
// none are specified.
func (opts *Docker) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(opts.Port < 0, "port must be positive value")
	catcher.NewWhen(opts.Image == "", "Docker image must be specified")
	if opts.Platform == "" {
		if PlatformSupportsDocker(runtime.GOOS) {
			opts.Platform = runtime.GOOS
		} else {
			catcher.Errorf("failed to set default platform to current runtime platform '%s' because it is unsupported", opts.Platform)
		}
	} else if !PlatformSupportsDocker(opts.Platform) {
		catcher.Errorf("unrecognized platform '%s'", opts.Platform)
	}
	return catcher.Resolve()
}

// Copy returns a copy of the options for only the exported fields.
func (opts *Docker) Copy() *Docker {
	optsCopy := *opts
	return &optsCopy
}

// PlatformSupportsDocker returns whether or not the platform has support for
// Docker.
func PlatformSupportsDocker(platform string) bool {
	switch platform {
	case "darwin", "linux", "windows":
		return true
	default:
		return false
	}
}

// Resolve converts the Docker options into options to initialize a Docker
// client.
func (opts *Docker) Resolve() (*client.Client, error) {
	var clientOpts []client.Opt

	if opts.Host != "" && opts.Port > 0 {
		addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("%s:%d", opts.Host, opts.Port))
		if err != nil {
			return nil, errors.Wrapf(err, "could not resolve Docker daemon address %s:%d", opts.Host, opts.Port)
		}
		clientOpts = append(clientOpts, client.WithHost(addr.String()))
	}

	if opts.APIVersion != "" {
		clientOpts = append(clientOpts, client.WithAPIVersionNegotiation())
	} else {
		clientOpts = append(clientOpts, client.WithVersion(opts.APIVersion))
	}

	client, err := client.NewClientWithOpts(clientOpts...)
	if err != nil {
		return nil, errors.Wrap(err, "could not create Docker client")
	}
	return client, nil
}
