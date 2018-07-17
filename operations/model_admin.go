package operations

import (
	"strings"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// ClientAdminConf tracks information about the evergreen deployment
// internally to support doing operations against specific app servers
// individually, including administering background jobs, and
// potentially self-orchestrating deploys.
type ClientAdminConf struct {
	Port       int      `json:"port" yaml:"port,omitempty"`
	AppServers []string `json:"app_servers" yaml:"app_servers,omitempty"`
}

// Validate provides very basic filtering of the admin configuration document.
func (c ClientAdminConf) Validate() error {
	catcher := grip.NewBasicCatcher()

	if c.Port < 1024 || c.Port == 8080 || c.Port == 9090 {
		catcher.Add(errors.Errorsf("'%d' is not a valid port number".c.Port))
	}

	for _, srv := range c.AppServers {
		if !strings.HasPrefix(srv, "http") {
			catcher.Add(errors.Errorf("invalid form for app server: %s", srv))
		}
	}

	return catcher.Resolve()
}
