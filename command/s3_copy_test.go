package command

import (
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestS3CopyPluginExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	comm := client.NewMock("http://localhost.com")

	testConfig := testutil.TestConfig()
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	testutil.ConfigureIntegrationTest(t, testConfig, "TestS3CopyPluginExecution")

	Convey("With a SimpleRegistry and test project file", t, func() {
		version := &version.Version{
			Id: "versionId",
		}
		So(version.Insert(), ShouldBeNil)

		pwd := testutil.GetDirectoryOfFile()
		configFile := filepath.Join(pwd, "testdata", "plugin_s3_copy.yml")
		modelData, err := modelutil.SetupAPITestData(testConfig, "test", "linux-64", configFile, modelutil.NoPatch)
		testutil.HandleTestingErr(err, t, "failed to setup test data")
		conf := modelData.TaskConfig
		conf.WorkDir = "."
		logger := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret})

		conf.Expansions.Update(map[string]string{
			"aws_key":    testConfig.Providers.AWS.Id,
			"aws_secret": testConfig.Providers.AWS.Secret,
			"pwd":        pwd,
		})

		Convey("the s3 copy command should execute successfully", func() {
			for _, task := range conf.Project.Tasks {
				for _, command := range task.Commands {
					pluginCmds, err := Render(command, conf.Project.Functions)
					testutil.HandleTestingErr(err, t, "Couldn't get plugin command: %v")
					So(pluginCmds, ShouldNotBeNil)
					So(err, ShouldBeNil)
					err = pluginCmds[0].Execute(ctx, comm, logger, conf)
					So(err, ShouldBeNil)
				}
			}
		})
	})
}
