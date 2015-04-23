package apiserver

import (
	"10gen.com/mci"
	"10gen.com/mci/plugin"
	"crypto/tls"
	"fmt"
	"net"
)

var port = 8181

type TestServer struct {
	URL string
	net.Listener
	*APIServer
}

func CreateTestServer(mciSettings *mci.MCISettings, tlsConfig *tls.Config, plugins []plugin.Plugin, verbose bool) (*TestServer, error) {
	apiServer, err := New(mciSettings, plugins)
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
