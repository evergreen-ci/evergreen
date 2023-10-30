package command

import (
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
)

func TestS3GetValidateParams(t *testing.T) {

	Convey("With an s3 get command", t, func() {

		var cmd *s3get

		Convey("when validating params", func() {

			cmd = &s3get{}

			Convey("a missing aws key should cause an error", func() {

				params := map[string]interface{}{
					"aws_secret":  "secret",
					"remote_file": "remote",
					"bucket":      "bck",
					"local_file":  "local",
				}
				So(cmd.ParseParams(params), ShouldNotBeNil)
				So(cmd.validateParams(), ShouldNotBeNil)
			})

			Convey("a missing aws secret should cause an error", func() {

				params := map[string]interface{}{
					"aws_key":     "key",
					"remote_file": "remote",
					"bucket":      "bck",
					"local_file":  "local",
				}
				So(cmd.ParseParams(params), ShouldNotBeNil)
				So(cmd.validateParams(), ShouldNotBeNil)

			})

			Convey("a missing remote file should cause an error", func() {

				params := map[string]interface{}{
					"aws_key":    "key",
					"aws_secret": "secret",
					"bucket":     "bck",
					"local_file": "local",
				}
				So(cmd.ParseParams(params), ShouldNotBeNil)
				So(cmd.validateParams(), ShouldNotBeNil)

			})

			Convey("a missing bucket should cause an error", func() {

				params := map[string]interface{}{
					"aws_key":     "key",
					"aws_secret":  "secret",
					"remote_file": "remote",
					"local_file":  "local",
				}
				So(cmd.ParseParams(params), ShouldNotBeNil)
				So(cmd.validateParams(), ShouldNotBeNil)

			})

			Convey("having neither a local file nor extract-to specified"+
				" should cause an error", func() {

				params := map[string]interface{}{
					"aws_key":     "key",
					"aws_secret":  "secret",
					"remote_file": "remote",
					"bucket":      "bck",
				}
				So(cmd.ParseParams(params), ShouldNotBeNil)
				So(cmd.validateParams(), ShouldNotBeNil)

			})

			Convey("having both a local file and an extract-to specified"+
				" should cause an error", func() {

				params := map[string]interface{}{
					"aws_key":     "key",
					"aws_secret":  "secret",
					"remote_file": "remote",
					"bucket":      "bck",
					"local_file":  "local",
					"extract_to":  "extract",
				}
				So(cmd.ParseParams(params), ShouldNotBeNil)
				So(cmd.validateParams(), ShouldNotBeNil)

			})

			Convey("a valid set of params should not cause an error", func() {

				params := map[string]interface{}{
					"aws_key":     "key",
					"aws_secret":  "secret",
					"remote_file": "remote",
					"bucket":      "bck",
					"local_file":  "local",
					"optional":    true,
				}
				So(cmd.ParseParams(params), ShouldBeNil)
				So(cmd.validateParams(), ShouldBeNil)

			})

		})

	})
}

func TestExpandS3GetParams(t *testing.T) {

	Convey("With an s3 get command and a task config", t, func() {

		var cmd *s3get
		var conf *internal.TaskConfig

		Convey("when expanding the command's params", func() {

			cmd = &s3get{}
			conf = &internal.TaskConfig{
				Expansions: *util.NewExpansions(map[string]string{}),
			}

			Convey("all appropriate values should be expanded, if they"+
				" contain expansions", func() {

				cmd.AwsKey = "${aws_key}"
				cmd.AwsSecret = "${aws_secret}"
				cmd.Bucket = "${bucket}"

				conf.Expansions.Update(
					map[string]string{
						"aws_key":    "key",
						"aws_secret": "secret",
					},
				)

				So(cmd.expandParams(conf), ShouldBeNil)
				So(cmd.AwsKey, ShouldEqual, "key")
				So(cmd.AwsSecret, ShouldEqual, "secret")
			})

		})

	})
}
