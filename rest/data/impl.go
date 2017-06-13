package data

// DBConnector is a struct that implements all of the methods which
// connect to the service layer of evergreen. These methods abstract the link
// between the service and the API layers, allowing for changes in the
// service architecture without forcing changes to the API.
type DBConnector struct {
	superUsers []string
	URL        string
	Prefix     string

	DBUserConnector
	DBTaskConnector
	DBContextConnector
	DBDistroConnector
	DBHostConnector
	DBTestConnector
	DBMetricsConnector
}

func (ctx *DBConnector) GetSuperUsers() []string   { return ctx.superUsers }
func (ctx *DBConnector) SetSuperUsers(su []string) { ctx.superUsers = su }
func (ctx *DBConnector) GetURL() string            { return ctx.URL }
func (ctx *DBConnector) SetURL(url string)         { ctx.URL = url }
func (ctx *DBConnector) GetPrefix() string         { return ctx.Prefix }
func (ctx *DBConnector) SetPrefix(prefix string)   { ctx.Prefix = prefix }

type MockConnector struct {
	superUsers []string
	URL        string
	Prefix     string

	MockUserConnector
	MockTaskConnector
	MockContextConnector
	MockDistroConnector
	MockHostConnector
	MockTestConnector
	MockMetricsConnector
}

func (ctx *MockConnector) GetSuperUsers() []string   { return ctx.superUsers }
func (ctx *MockConnector) SetSuperUsers(su []string) { ctx.superUsers = su }
func (ctx *MockConnector) GetURL() string            { return ctx.URL }
func (ctx *MockConnector) SetURL(url string)         { ctx.URL = url }
func (ctx *MockConnector) GetPrefix() string         { return ctx.Prefix }
func (ctx *MockConnector) SetPrefix(prefix string)   { ctx.Prefix = prefix }
