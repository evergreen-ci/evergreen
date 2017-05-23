package testutil

import (
	"fmt"
	"net"
	"net/http/httptest"

	"github.com/codegangsta/negroni"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/route"
	"github.com/evergreen-ci/evergreen/rest/servicecontext"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
)

const (
	testServerPort = 9191
)

// NewTestServerFromSettings takes an evergreen.Settings and creates a database backed
// REST v2 test server. It automatically starts the server on port 9191.
func NewTestServerFromSettings(settings *evergreen.Settings) (*httptest.Server, error) {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(settings))
	sc := &servicecontext.DBServiceContext{}

	sc.SetPrefix(evergreen.RestRoutePrefix)
	sc.SetSuperUsers(settings.SuperUsers)

	return NewTestServerFromServiceContext(testServerPort, sc)
}

// NewTestServerFromServiceContext takes in a port and already constructed ServiceContext
// and creates an REST v2 API server. This is very useful when testing, especially when
// mocking out sections of the ServiceContext to make sure request occur as expected.
func NewTestServerFromServiceContext(port int, sc servicecontext.ServiceContext) (*httptest.Server, error) {
	root := mux.NewRouter()
	route.GetHandler(root, sc)
	n := negroni.New()
	n.UseHandler(root)

	server := httptest.NewUnstartedServer(n)
	addr := fmt.Sprintf(":%d", port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	sc.SetURL(fmt.Sprintf("http://localhost:%d", port))
	server.Listener = l
	server.Start()

	grip.Infoln("started server:", sc.GetURL())

	return server, nil
}
