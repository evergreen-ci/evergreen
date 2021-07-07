package data

// DBConnector is a struct that implements all of the methods which
// connect to the service layer of evergreen. These methods abstract the link
// between the service and the API layers, allowing for changes in the
// service architecture without forcing changes to the API.
type DBConnector struct {
	URL    string
	Prefix string

	DBUserConnector
	DBTaskConnector
	DBContextConnector
	DBDistroConnector
	DBHostConnector
	DBPodConnector
	DBTestConnector
	DBBuildConnector
	DBVersionConnector
	DBPatchConnector
	DBPatchIntentConnector
	DBProjectConnector
	DBAdminConnector
	DBStatusConnector
	DBAliasConnector
	RepoTrackerConnector
	CLIUpdateConnector
	GenerateConnector
	DBSubscriptionConnector
	NotificationConnector
	DBCreateHostConnector
	StatsConnector
	TaskReliabilityConnector
	DBCommitQueueConnector
	SchedulerConnector
}

func (ctx *DBConnector) GetURL() string          { return ctx.URL }
func (ctx *DBConnector) SetURL(url string)       { ctx.URL = url }
func (ctx *DBConnector) GetPrefix() string       { return ctx.Prefix }
func (ctx *DBConnector) SetPrefix(prefix string) { ctx.Prefix = prefix }

type MockConnector struct {
	URL    string
	Prefix string

	MockUserConnector
	MockTaskConnector
	MockContextConnector
	MockDistroConnector
	MockHostConnector
	MockPodConnector
	MockTestConnector
	MockBuildConnector
	MockVersionConnector
	MockPatchConnector
	MockPatchIntentConnector
	MockProjectConnector
	MockAdminConnector
	MockStatusConnector
	MockAliasConnector
	MockRepoTrackerConnector
	MockCLIUpdateConnector
	MockGenerateConnector
	MockSubscriptionConnector
	MockNotificationConnector
	MockCreateHostConnector
	MockStatsConnector
	MockTaskReliabilityConnector
	MockCommitQueueConnector
	MockSchedulerConnector
}

func (ctx *MockConnector) GetURL() string          { return ctx.URL }
func (ctx *MockConnector) SetURL(url string)       { ctx.URL = url }
func (ctx *MockConnector) GetPrefix() string       { return ctx.Prefix }
func (ctx *MockConnector) SetPrefix(prefix string) { ctx.Prefix = prefix }
