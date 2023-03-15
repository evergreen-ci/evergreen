package data

// DBConnector is a struct that implements all of the methods which
// connect to the service layer of evergreen. These methods abstract the link
// between the service and the API layers, allowing for changes in the
// service architecture without forcing changes to the API. This is only
// required for the methods that must be mocked in unit tests, since they
// need a mocked implementation and a real DB implementation of the interface.
type DBConnector struct {
	URL    string
	Prefix string
	DBCommitQueueConnector
	DBProjectConnector
	DBVersionConnector
	DBGithubConnector
}

func (ctx *DBConnector) GetURL() string          { return ctx.URL }
func (ctx *DBConnector) SetURL(url string)       { ctx.URL = url }
func (ctx *DBConnector) GetPrefix() string       { return ctx.Prefix }
func (ctx *DBConnector) SetPrefix(prefix string) { ctx.Prefix = prefix }

type MockGitHubConnector struct {
	URL string
	DBConnector
	MockGitHubConnectorImpl
}

func (ctx *MockGitHubConnector) GetURL() string { return ctx.URL }
