package gimlet

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

// Server provides a wrapper around *http.Server methods, with an
// Run method which provides a clean-shutdown compatible listen operation.
type Server interface {
	Close() error
	ListenAndServe() error
	ListenAndServeTLS(string, string) error
	Serve(net.Listener) error
	Shutdown(context.Context) error

	// Run provides a simple wrapper around default http.Server
	// functionality to allow clean shutdown that uses a context
	// with a wait function
	Run(context.Context) (WaitFunc, error)
	// GetServer allows you to access the underlying http server.
	GetServer() *http.Server
}

// TODO add ServeTLS when we can move up to 1.9+:
//
// ServeTLS(net.Listener, string, string) error

type ServerConfig struct {
	Timeout time.Duration
	Handler http.Handler
	App     *APIApp
	TLS     *tls.Config
	Address string
	Info    string

	handlerGenerated bool
}

// Validate returns an error if there are any problems with a
// ServerConfig that would make it impossible to render a server from
// the configuration.
func (c *ServerConfig) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(c.TLS != nil && c.TLS.Certificates == nil, "tls config specified without certificates")
	catcher.NewWhen(c.Handler == nil && c.App == nil, "must specify a handler or a gimlet app")
	catcher.NewWhen(c.Handler != nil && c.App != nil && !c.handlerGenerated, "can only specify a handler or an app")
	catcher.NewWhen(c.Address == "", "must specify an address")
	catcher.ErrorfWhen(c.Timeout < time.Second, "must specify timeout greater than a second, '%s'", c.Timeout)
	catcher.ErrorfWhen(c.Timeout > 10*time.Minute, "must specify timeout less than 10 minutes, '%s'", c.Timeout)

	_, _, err := net.SplitHostPort(c.Address)
	catcher.Add(err)

	if c.App != nil {
		c.Handler, err = c.App.Handler()
		catcher.Add(err)
		c.handlerGenerated = true
	}

	return catcher.Resolve()
}

func (c *ServerConfig) build() Server {
	return server{&http.Server{
		Addr:              c.Address,
		Handler:           c.Handler,
		ReadTimeout:       c.Timeout,
		ReadHeaderTimeout: c.Timeout / 2,
		WriteTimeout:      c.Timeout,
		TLSConfig:         c.TLS,
	}}
}

// Resovle validates a config and constructs a server from the
// configuration if possible.
func (c *ServerConfig) Resolve() (Server, error) {
	if err := c.Validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	return c.build(), nil
}

// NewServer constructs a server based on a handler and an
// address. Uses a default 1 minute timeout.
func NewServer(addr string, n http.Handler) (Server, error) {
	conf := ServerConfig{
		Timeout: time.Minute,
		Handler: n,
		Address: addr,
	}

	return conf.Resolve()
}

// BuildNewServer constructs a new server that uses TLS, returning an
// error if you pass a nil TLS configuration.
func BuildNewServer(addr string, n http.Handler, tlsConf *tls.Config) (Server, error) {
	if tlsConf == nil {
		return nil, errors.New("must specify a non-nil tls config")
	}

	conf := ServerConfig{
		Address: addr,
		Handler: n,
		TLS:     tlsConf,
		Timeout: time.Minute,
	}

	return conf.Resolve()
}

type server struct {
	*http.Server
}

func (s server) GetServer() *http.Server { return s.Server }

func (s server) Run(ctx context.Context) (WaitFunc, error) {
	serviceWait := make(chan struct{})
	go func() {
		defer recovery.LogStackTraceAndContinue("app service")
		if s.Server.TLSConfig != nil {
			grip.Error(errors.Wrap(s.ListenAndServeTLS("", ""), "problem starting tls service"))
		} else {
			grip.Error(errors.Wrap(s.ListenAndServe(), "problem starting service"))
		}

		close(serviceWait)
	}()

	go func() {
		defer recovery.LogStackTraceAndContinue("server shutdown")
		<-ctx.Done()
		grip.Debug(s.Shutdown(ctx))
	}()

	wait := func(wctx context.Context) {
		select {
		case <-wctx.Done():
		case <-serviceWait:
		}
	}

	return wait, nil
}
