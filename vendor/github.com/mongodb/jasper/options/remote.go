package options

import (
	"fmt"

	"github.com/pkg/errors"
)

// Remote represents options to SSH into a remote machine.
type Remote struct {
	Host string
	User string
	Args []string
}

// Validate checks that the host is set so that the remote host can be
// identified.
func (opts *Remote) Validate() error {
	if opts.Host == "" {
		return errors.New("host cannot be empty")
	}
	return nil
}

func (opts *Remote) String() string {
	if opts.User == "" {
		return opts.Host
	}

	return fmt.Sprintf("%s@%s", opts.User, opts.Host)
}
