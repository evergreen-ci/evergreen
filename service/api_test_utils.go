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
	"github.com/urfave/negroni"
)

type TestServer struct {
	URL string
	net.Listener
	*APIServer
	ts *httptest.Server
}

func (s *TestServer) Close() {
	grip.Alertln("closing test server:", s.URL)

	grip.CatchError(s.Listener.Close())
	s.ts.CloseClientConnections()
	s.ts.Close()
}

func CreateTestServer(settings *evergreen.Settings, tlsConfig *tls.Config) (*TestServer, error) {
	port := testutil.NextPort()
	if err := os.MkdirAll(filepath.Join(evergreen.FindEvergreenHome(), evergreen.ClientDirectory), 0644); err != nil {
		return nil, err
	}

	as, err := NewAPIServer(settings)
	if err != nil {
		return nil, err
	}
	as.UserManager = testutil.MockUserManager{}

	var l net.Listener
	protocol := "http"

	n := negroni.New()
	n.Use(NewLogger())
	n.Use(negroni.HandlerFunc(UserMiddleware(as.UserManager)))
	n.UseHandler(as.NewRouter())

	server := httptest.NewUnstartedServer(n)
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
