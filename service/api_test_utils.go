package service

import (
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/plugin"
)

var port = 8181

type TestServer struct {
	URL string
	net.Listener
	*APIServer
}

func CreateTestServer(settings *evergreen.Settings, tlsConfig *tls.Config, plugins []plugin.APIPlugin, verbose bool) (*TestServer, error) {
	if err := os.MkdirAll(filepath.Join(evergreen.FindEvergreenHome(), evergreen.ClientDirectory), 0644); err != nil {
		return nil, err
	}

	apiServer, err := NewAPIServer(settings, plugins)
	if err != nil {
		return nil, err
	}
	var l net.Listener
	protocol := "http"
	port++
	addr := fmt.Sprintf(":%v", port)

	if tlsConfig == nil {
		l, err = GetListener(addr)
		if err != nil {
			return nil, err
		}
	} else {
		l, err = GetTLSListener(addr, tlsConfig)
		if err != nil {
			return nil, err
		}
		protocol = "https"
	}

	h, err := apiServer.Handler()
	if err != nil {
		return nil, err
	}
	go Serve(l, h)

	return &TestServer{fmt.Sprintf("%s://localhost%v", protocol, addr), l, apiServer}, nil
}
