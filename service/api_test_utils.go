package service

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/service/testutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// GetListener creates a network listener on the given address.
func GetListener(addr string) (net.Listener, error) {
	return net.Listen("tcp", addr)
}

// GetTLSListener creates an encrypted listener with the given TLS config and address.
func GetTLSListener(addr string, conf *tls.Config) (net.Listener, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return tls.NewListener(l, conf), nil
}

type TestServer struct {
	URL string
	net.Listener
	*APIServer
	ts *httptest.Server
}

func (s *TestServer) Close() {
	grip.Noticeln("closing test server:", s.URL)

	grip.Error(s.Listener.Close())
	s.ts.CloseClientConnections()
	s.ts.Close()
}

func CreateTestServer(settings *evergreen.Settings, tlsConfig *tls.Config) (*TestServer, error) {
	home := evergreen.FindEvergreenHome()
	port := testutil.NextPort()
	if err := os.MkdirAll(filepath.Join(home, evergreen.ClientDirectory), 0644); err != nil {
		return nil, err
	}

	env := evergreen.GetEnvironment()

	as, err := NewAPIServer(settings, env.LocalQueue(), env.RemoteQueueGroup())
	if err != nil {
		return nil, err
	}
	as.UserManager = testutil.MockUserManager{}

	uis, err := NewUIServer(settings, env.LocalQueue(), home, TemplateFunctionOptions{})
	if err != nil {
		return nil, err
	}
	uis.UserManager = testutil.MockUserManager{}

	var l net.Listener
	protocol := "http"

	handler, err := GetRouter(as, uis)
	if err != nil {
		return nil, err
	}

	server := httptest.NewUnstartedServer(handler)
	server.TLS = tlsConfig

	// We're not running ssl tests with the agent in any cases,
	// but currently its set up to clients of this test server
	// should figure out the port from the TestServer instance's
	// URL field.
	//
	// We try and make sure that the SSL servers on different
	// ports than their non-ssl servers.

	var addr string
	if tlsConfig == nil {
		addr = fmt.Sprintf(":%d", port)
		l, err = GetListener(addr)
		if err != nil {
			return nil, err
		}
		server.Listener = l
		go server.Start()
	} else {
		addr = fmt.Sprintf(":%d", port+1)
		l, err = GetTLSListener(addr, tlsConfig)
		if err != nil {
			return nil, err
		}
		protocol = "https"
		server.Listener = l
		go server.StartTLS()
	}

	ts := &TestServer{
		URL:       fmt.Sprintf("%s://localhost%v", protocol, addr),
		Listener:  l,
		APIServer: as,
		ts:        server,
	}

	grip.Infoln("started server:", ts.URL)

	return ts, nil
}
