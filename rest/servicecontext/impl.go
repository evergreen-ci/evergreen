package servicecontext

// DBServiceContext is a struct that implements all of the methods which
// connect to the service layer of evergreen. These methods abstract the link
// between the service and the API layers, allowing for changes in the
// service architecture without forcing changes to the API.
type DBServiceContext struct {
	superUsers []string
	URL        string
	Prefix     string

	DBUserConnector
	DBTaskConnector
	DBContextConnector
	DBHostConnector
	DBTestConnector
}

func (ctx *DBServiceContext) GetSuperUsers() []string   { return ctx.superUsers }
func (ctx *DBServiceContext) SetSuperUsers(su []string) { ctx.superUsers = su }
func (ctx *DBServiceContext) GetURL() string            { return ctx.URL }
func (ctx *DBServiceContext) SetURL(url string)         { ctx.URL = url }
func (ctx *DBServiceContext) GetPrefix() string         { return ctx.Prefix }
func (ctx *DBServiceContext) SetPrefix(prefix string)   { ctx.Prefix = prefix }

type MockServiceContext struct {
	superUsers []string
	URL        string
	Prefix     string

	MockUserConnector
	MockTaskConnector
	MockContextConnector
	MockHostConnector
	MockTestConnector
}

func (ctx *MockServiceContext) GetSuperUsers() []string   { return ctx.superUsers }
func (ctx *MockServiceContext) SetSuperUsers(su []string) { ctx.superUsers = su }
func (ctx *MockServiceContext) GetURL() string            { return ctx.URL }
func (ctx *MockServiceContext) SetURL(url string)         { ctx.URL = url }
func (ctx *MockServiceContext) GetPrefix() string         { return ctx.Prefix }
func (ctx *MockServiceContext) SetPrefix(prefix string)   { ctx.Prefix = prefix }
