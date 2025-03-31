package command

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestS3GetValidateParams(t *testing.T) {

	Convey("With an s3 get command", t, func() {

		var cmd *s3get

		Convey("when validating params", func() {

			cmd = &s3get{}

			Convey("a missing aws key should cause an error", func() {

				params := map[string]any{
					"aws_secret":  "secret",
					"remote_file": "remote",
					"bucket":      "bck",
					"local_file":  "local",
				}
				So(cmd.ParseParams(params), ShouldNotBeNil)
				So(cmd.validate(), ShouldNotBeNil)
			})

			Convey("a missing aws secret should cause an error", func() {

				params := map[string]any{
					"aws_key":     "key",
					"remote_file": "remote",
					"bucket":      "bck",
					"local_file":  "local",
				}
				So(cmd.ParseParams(params), ShouldNotBeNil)
				So(cmd.validate(), ShouldNotBeNil)

			})

			Convey("a missing remote file should cause an error", func() {

				params := map[string]any{
					"aws_key":    "key",
					"aws_secret": "secret",
					"bucket":     "bck",
					"local_file": "local",
				}
				So(cmd.ParseParams(params), ShouldNotBeNil)
				So(cmd.validate(), ShouldNotBeNil)

			})

			Convey("a missing bucket should cause an error", func() {

				params := map[string]any{
					"aws_key":     "key",
					"aws_secret":  "secret",
					"remote_file": "remote",
					"local_file":  "local",
				}
				So(cmd.ParseParams(params), ShouldNotBeNil)
				So(cmd.validate(), ShouldNotBeNil)

			})

			Convey("having neither a local file nor extract-to specified"+
				" should cause an error", func() {

				params := map[string]any{
					"aws_key":     "key",
					"aws_secret":  "secret",
					"remote_file": "remote",
					"bucket":      "bck",
				}
				So(cmd.ParseParams(params), ShouldNotBeNil)
				So(cmd.validate(), ShouldNotBeNil)

			})

			Convey("having both a local file and an extract-to specified"+
				" should cause an error", func() {

				params := map[string]any{
					"aws_key":     "key",
					"aws_secret":  "secret",
					"remote_file": "remote",
					"bucket":      "bck",
					"local_file":  "local",
					"extract_to":  "extract",
				}
				So(cmd.ParseParams(params), ShouldNotBeNil)
				So(cmd.validate(), ShouldNotBeNil)

			})

			Convey("a valid set of params should not cause an error", func() {

				params := map[string]any{
					"aws_key":     "key",
					"aws_secret":  "secret",
					"remote_file": "remote",
					"bucket":      "bck",
					"local_file":  "local",
					"optional":    true,
				}
				So(cmd.ParseParams(params), ShouldBeNil)
				So(cmd.validate(), ShouldBeNil)

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

func writeSingleFileArchive(path, filename string, data []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return errors.Wrapf(err, "creating file")
	}

	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	header := &tar.Header{
		Typeflag: tar.TypeReg,
		Name:     filename,
		Size:     int64(len(data)),
		Mode:     0777,
	}

	if err := tw.WriteHeader(header); err != nil {
		return errors.Wrapf(err, "writing header")
	}

	if _, err := io.Copy(tw, bytes.NewReader(data)); err != nil {
		return errors.Wrapf(err, "copying data")
	}

	return nil
}

func TestS3GetFetchesFiles(t *testing.T) {
	if skip, _ := strconv.ParseBool(os.Getenv("SKIP_INTEGRATION_TESTS")); skip {
		t.Skip("SKIP_INTEGRATION_TESTS is set, skipping integration test")
	}

	temproot := t.TempDir()

	settings := testutil.GetIntegrationFile(t)

	payload := []byte("hello world")

	accessKeyID := settings.Expansions["aws_key"]
	secretAccessKey := settings.Expansions["aws_secret"]
	token := settings.Expansions["aws_token"]
	bucketName := settings.Expansions["bucket"]
	region := "us-east-1"
	sender := send.MakeInternalLogger()
	logger := client.NewSingleChannelLogHarness("test", sender)
	comm := client.NewMock("")

	id := utility.RandomString()

	ctx := t.Context()

	tconf := &internal.TaskConfig{
		Task:    task.Task{},
		WorkDir: temproot,
	}

	t.Run("GetOptionalDoesNotError", func(t *testing.T) {
		getCommand := s3GetFactory()
		getParams := map[string]any{
			"aws_key":           accessKeyID,
			"aws_secret":        secretAccessKey,
			"aws_session_token": token,
			"local_file":        "testfile",
			"remote_file":       "abc123",
			"bucket":            bucketName,
			"region":            region,
			"optional":          "true",
		}

		require.NoError(t, getCommand.ParseParams(getParams))
		require.NoError(t, getCommand.Execute(ctx, comm, logger, tconf))
	})

	t.Run("GetPlainFile", func(t *testing.T) {
		remoteFile := fmt.Sprintf("tests/%s/%s", t.Name(), id)
		putFilePath := filepath.Join(temproot, "upload-file.txt")
		getFilePath := filepath.Join(temproot, "download-file.txt")

		require.NoError(t, os.WriteFile(putFilePath, payload, 0755))

		putCommand := s3PutFactory()
		putParams := map[string]any{
			"aws_key":           accessKeyID,
			"aws_secret":        secretAccessKey,
			"aws_session_token": token,
			"local_file":        putFilePath,
			"remote_file":       remoteFile,
			"bucket":            bucketName,
			"region":            region,
			"skip_existing":     "true",
			"content_type":      "text/plain",
			"permissions":       "private",
		}

		require.NoError(t, putCommand.ParseParams(putParams))
		require.NoError(t, putCommand.Execute(ctx, comm, logger, tconf))

		getCommand := s3GetFactory()
		getParams := map[string]any{
			"aws_key":           accessKeyID,
			"aws_secret":        secretAccessKey,
			"aws_session_token": token,
			"local_file":        getFilePath,
			"remote_file":       remoteFile,
			"bucket":            bucketName,
			"region":            region,
		}

		require.NoError(t, getCommand.ParseParams(getParams))
		require.NoError(t, getCommand.Execute(ctx, comm, logger, tconf))

		b, err := os.ReadFile(getFilePath)
		require.NoError(t, err)

		assert.Equal(t, b, payload)
	})

	t.Run("GetTarFile", func(t *testing.T) {
		remoteTarFile := fmt.Sprintf("tests/%s/%s.tgz", t.Name(), id)
		putTarFilePath := filepath.Join(temproot, "upload-file.tgz")
		tarFileName := "hello-world.txt"
		extractToFilePath := filepath.Join(temproot, "extract-to")
		require.NoError(t, writeSingleFileArchive(putTarFilePath, tarFileName, payload))

		putTarCommand := s3PutFactory()
		putTarParams := map[string]any{
			"aws_key":           accessKeyID,
			"aws_secret":        secretAccessKey,
			"aws_session_token": token,
			"local_file":        putTarFilePath,
			"remote_file":       remoteTarFile,
			"bucket":            bucketName,
			"region":            region,
			"skip_existing":     "true",
			"content_type":      "text/plain",
			"permissions":       "private",
		}

		require.NoError(t, putTarCommand.ParseParams(putTarParams))
		require.NoError(t, putTarCommand.Execute(ctx, comm, logger, tconf))

		getTarCommand := s3GetFactory()
		getTarParams := map[string]any{
			"aws_key":           accessKeyID,
			"aws_secret":        secretAccessKey,
			"aws_session_token": token,
			"extract_to":        extractToFilePath,
			"remote_file":       remoteTarFile,
			"bucket":            bucketName,
			"region":            region,
		}

		require.NoError(t, getTarCommand.ParseParams(getTarParams))
		require.NoError(t, getTarCommand.Execute(ctx, comm, logger, tconf))

		tarb, err := os.ReadFile(filepath.Join(extractToFilePath, tarFileName))
		require.NoError(t, err)

		assert.Equal(t, tarb, payload)
	})
}
