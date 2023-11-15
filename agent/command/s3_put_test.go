package command

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/pail"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestS3PutValidateParams(t *testing.T) {

	Convey("With an s3 put command", t, func() {

		var cmd *s3put

		Convey("when validating command params", func() {

			cmd = &s3put{}

			Convey("a missing aws key should cause an error", func() {

				params := map[string]interface{}{
					"aws_secret":   "secret",
					"local_file":   "local",
					"remote_file":  "remote",
					"bucket":       "bck",
					"permissions":  "public-read",
					"content_type": "application/x-tar",
					"display_name": "test_file",
				}
				err := cmd.ParseParams(params)
				require.Error(t, err)
				So(err.Error(), ShouldContainSubstring, "AWS key cannot be blank")
			})
			Convey("a defined local file and inclusion filter should cause an error", func() {

				params := map[string]interface{}{
					"aws_secret":                 "secret",
					"aws_key":                    "key",
					"local_file":                 "local",
					"local_files_include_filter": []string{"local"},
					"remote_file":                "remote",
					"bucket":                     "bck",
					"permissions":                "public-read",
					"content_type":               "application/x-tar",
					"display_name":               "test_file",
				}
				err := cmd.ParseParams(params)
				require.Error(t, err)
				So(err.Error(), ShouldContainSubstring, "local file and local files include filter cannot both be specified")
			})
			Convey("a defined inclusion filter with optional upload should cause an error", func() {

				params := map[string]interface{}{
					"aws_secret":                 "secret",
					"aws_key":                    "key",
					"local_files_include_filter": []string{"local"},
					"optional":                   true,
					"remote_file":                "remote",
					"bucket":                     "bck",
					"permissions":                "public-read",
					"content_type":               "application/x-tar",
					"display_name":               "test_file",
				}
				// Before the optional parameter is expanded, we expect no errors.
				err := cmd.ParseParams(params)
				require.NoError(t, err)
				// Then we have to expand the parameters.
				abs, err := filepath.Abs("working_directory")
				require.NoError(t, err)
				conf := &internal.TaskConfig{
					Expansions: *util.NewExpansions(map[string]string{}),
					WorkDir:    abs,
				}
				err = cmd.expandParams(conf)
				require.NoError(t, err)
				// Now that optional is expanded to skip missing, we expect an error.
				err = cmd.ParseParams(params)
				require.Error(t, err)
				So(err.Error(), ShouldContainSubstring, "cannot use optional with local files include filter as by default it is optional")
			})

			Convey("a missing aws secret should cause an error", func() {

				params := map[string]interface{}{
					"aws_key":      "key",
					"local_file":   "local",
					"remote_file":  "remote",
					"bucket":       "bck",
					"permissions":  "public-read",
					"content_type": "application/x-tar",
					"display_name": "test_file",
				}
				err := cmd.ParseParams(params)
				require.Error(t, err)
				So(err.Error(), ShouldContainSubstring, "AWS secret cannot be blank")
			})

			Convey("a missing local file should cause an error", func() {

				params := map[string]interface{}{
					"aws_key":      "key",
					"aws_secret":   "secret",
					"remote_file":  "remote",
					"bucket":       "bck",
					"permissions":  "public-read",
					"content_type": "application/x-tar",
					"display_name": "test_file",
				}
				err := cmd.ParseParams(params)
				require.Error(t, err)
				So(err.Error(), ShouldContainSubstring, "local file and local files include filter cannot both be blank")
			})

			Convey("a missing remote file should cause an error", func() {

				params := map[string]interface{}{
					"aws_key":      "key",
					"aws_secret":   "secret",
					"local_file":   "local",
					"bucket":       "bck",
					"permissions":  "public-read",
					"content_type": "application/x-tar",
					"display_name": "test_file",
				}
				err := cmd.ParseParams(params)
				require.Error(t, err)
				So(err.Error(), ShouldContainSubstring, "remote file cannot be blank")
			})

			Convey("a missing bucket should cause an error", func() {

				params := map[string]interface{}{
					"aws_key":      "key",
					"aws_secret":   "secret",
					"local_file":   "local",
					"remote_file":  "remote",
					"permissions":  "public-read",
					"content_type": "application/x-tar",
					"display_name": "test_file",
				}
				err := cmd.ParseParams(params)
				require.Error(t, err)
				So(err.Error(), ShouldContainSubstring, "invalid bucket name")
			})

			Convey("a missing s3 permission should cause an error", func() {

				params := map[string]interface{}{
					"aws_key":      "key",
					"aws_secret":   "secret",
					"local_file":   "local",
					"remote_file":  "remote",
					"bucket":       "bck",
					"content_type": "application/x-tar",
					"display_name": "test_file",
				}
				err := cmd.ParseParams(params)
				require.Error(t, err)
				So(err.Error(), ShouldContainSubstring, "invalid permissions")
			})

			Convey("an invalid s3 permission should cause an error", func() {

				params := map[string]interface{}{
					"aws_key":      "key",
					"aws_secret":   "secret",
					"local_file":   "local",
					"remote_file":  "remote",
					"bucket":       "bck",
					"permissions":  "bleccchhhh",
					"content_type": "application/x-tar",
					"display_name": "test_file",
				}
				err := cmd.ParseParams(params)
				require.Error(t, err)
				So(err.Error(), ShouldContainSubstring, "invalid permissions")
			})

			Convey("a missing content type should cause an error", func() {

				params := map[string]interface{}{
					"aws_key":      "key",
					"aws_secret":   "secret",
					"local_file":   "local",
					"remote_file":  "remote",
					"bucket":       "bck",
					"permissions":  "private",
					"display_name": "test_file",
				}
				err := cmd.ParseParams(params)
				require.Error(t, err)
				So(err.Error(), ShouldContainSubstring, "content type cannot be blank")
			})

			Convey("an invalid visibility type should cause an error", func() {

				params := map[string]interface{}{
					"aws_key":      "key",
					"aws_secret":   "secret",
					"local_file":   "local",
					"remote_file":  "remote",
					"bucket":       "bck",
					"content_type": "application/x-tar",
					"permissions":  "private",
					"display_name": "test_file",
					"visibility":   "ARGHGHGHGHGH",
				}
				err := cmd.ParseParams(params)
				require.Error(t, err)
				So(err.Error(), ShouldContainSubstring, "invalid visibility setting")
			})

			Convey("a valid set of params should not cause an error", func() {

				params := map[string]interface{}{
					"aws_key":      "key",
					"aws_secret":   "secret",
					"local_file":   "local",
					"remote_file":  "remote",
					"bucket":       "bck",
					"permissions":  "public-read",
					"content_type": "application/x-tar",
					"display_name": "test_file",
				}
				So(cmd.ParseParams(params), ShouldBeNil)
				So(cmd.AwsKey, ShouldEqual, params["aws_key"])
				So(cmd.AwsSecret, ShouldEqual, params["aws_secret"])
				So(cmd.LocalFile, ShouldEqual, params["local_file"])
				So(cmd.RemoteFile, ShouldEqual, params["remote_file"])
				So(cmd.Bucket, ShouldEqual, params["bucket"])
				So(cmd.Permissions, ShouldEqual, params["permissions"])
				So(cmd.ResourceDisplayName, ShouldEqual, params["display_name"])
			})
		})

	})
}

func TestExpandS3PutParams(t *testing.T) {

	Convey("With an s3 put command and a task config", t, func() {
		abs, err := filepath.Abs("working_directory")
		So(err, ShouldBeNil)

		cmd := &s3put{}
		conf := &internal.TaskConfig{
			Expansions: *util.NewExpansions(map[string]string{}),
			WorkDir:    abs,
		}

		Convey("when expanding the command's params all appropriate values should be expanded, if they"+
			" contain expansions", func() {

			cmd.AwsKey = "${aws_key}"
			cmd.AwsSecret = "${aws_secret}"
			cmd.RemoteFile = "${remote_file}"
			cmd.Bucket = "${bucket}"
			cmd.ContentType = "${content_type}"
			cmd.ResourceDisplayName = "${display_name}"
			cmd.Visibility = "${visibility}"
			cmd.Optional = "${optional}"
			cmd.LocalFile = abs

			conf.Expansions.Update(
				map[string]string{
					"aws_key":      "key",
					"aws_secret":   "secret",
					"remote_file":  "remote",
					"bucket":       "bck",
					"content_type": "ct",
					"display_name": "file",
					"optional":     "true",
					"visibility":   artifact.Private,
					"workdir":      "/working_directory",
				},
			)

			So(cmd.expandParams(conf), ShouldBeNil)
			So(cmd.AwsKey, ShouldEqual, "key")
			So(cmd.AwsSecret, ShouldEqual, "secret")
			So(cmd.RemoteFile, ShouldEqual, "remote")
			So(cmd.Bucket, ShouldEqual, "bck")
			So(cmd.ContentType, ShouldEqual, "ct")
			So(cmd.ResourceDisplayName, ShouldEqual, "file")
			So(cmd.Visibility, ShouldEqual, "private")
			So(cmd.Optional, ShouldEqual, "true")

			// EVG-7226 Since LocalFile is an absolute path, workDir should be empty
			So(cmd.workDir, ShouldEqual, "")
		})

		Convey("the expandParams function should error for invalid optional values", func() {
			cmd = &s3put{}

			for _, v := range []string{"", "false", "False", "0", "F", "f", "${foo|false}", "${foo|}", "${foo}"} {
				cmd.SkipExisting = "true"
				cmd.Optional = v
				So(cmd.expandParams(conf), ShouldBeNil)
				So(cmd.skipMissing, ShouldBeFalse)
			}

			for _, v := range []string{"true", "True", "1", "T", "t", "${foo|true}"} {
				cmd.skipMissing = false
				cmd.Optional = v
				So(cmd.expandParams(conf), ShouldBeNil)
				So(cmd.skipMissing, ShouldBeTrue)
			}

			for _, v := range []string{"NOPE", "NONE", "EMPTY", "01", "100", "${foo|wat}"} {
				cmd.Optional = v
				So(cmd.expandParams(conf), ShouldNotBeNil)
				So(cmd.skipMissing, ShouldBeFalse)
			}

		})

	})
}

func TestSignedUrlVisibility(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, vis := range []string{"signed", "private"} {
		s := s3put{
			AwsKey:        "key",
			AwsSecret:     "secret",
			Bucket:        "bucket",
			BuildVariants: []string{},
			ContentType:   "content-type",
			Permissions:   s3.BucketCannedACLPublicRead,
			RemoteFile:    "remote",
			Visibility:    vis,
		}

		comm := client.NewMock("http://localhost.com")
		conf := &internal.TaskConfig{
			Expansions:   util.Expansions{},
			Task:         task.Task{Id: "mock_id", Secret: "mock_secret"},
			Project:      model.Project{},
			BuildVariant: model.BuildVariant{},
		}
		logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
		require.NoError(t, err)

		localFiles := []string{"file1", "file2"}
		remoteFile := "remote file"

		require.NoError(t, s.attachFiles(ctx, comm, logger, localFiles, remoteFile))

		attachedFiles := comm.AttachedFiles
		if v, found := attachedFiles[""]; found {
			for _, file := range v {
				assert.NotEqual(t, " ", string(file.Name[0]))
				if file.Visibility == artifact.Signed {
					assert.Equal(t, file.AwsKey, s.AwsKey)
					assert.Equal(t, file.AwsSecret, s.AwsSecret)

				} else {
					assert.Equal(t, file.AwsKey, "")
					assert.Equal(t, file.AwsSecret, "")
				}
			}
		}
	}
}

func TestContentTypeSaved(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := s3put{
		AwsKey:        "key",
		AwsSecret:     "secret",
		Bucket:        "bucket",
		BuildVariants: []string{},
		ContentType:   "content-type",
		Permissions:   s3.BucketCannedACLPublicRead,
		RemoteFile:    "remote",
		Visibility:    "",
	}

	comm := client.NewMock("http://localhost.com")
	conf := &internal.TaskConfig{
		Expansions:   util.Expansions{},
		Task:         task.Task{Id: "mock_id", Secret: "mock_secret"},
		Project:      model.Project{},
		BuildVariant: model.BuildVariant{},
	}
	s.taskdata = client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
	require.NoError(t, err)

	localFiles := []string{"file1", "file2"}
	remoteFile := "remote file"

	require.NoError(t, s.attachFiles(ctx, comm, logger, localFiles, remoteFile))

	attachedFiles := comm.AttachedFiles
	files, ok := attachedFiles[conf.Task.Id]
	// Must be able to find an attached file
	require.True(t, ok)
	assert.Len(t, files, 2)
	for _, file := range files {
		assert.Equal(t, file.ContentType, s.ContentType)
	}

}

func TestS3LocalFilesIncludeFilterPrefix(t *testing.T) {
	for _, prefix := range []string{"emptyPrefix", "subDir"} {
		t.Run(prefix, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var err error

			dir := t.TempDir()
			f, err := os.Create(filepath.Join(dir, "foo"))
			require.NoError(t, err)
			require.NoError(t, f.Close())
			require.NoError(t, os.Mkdir(filepath.Join(dir, "subDir"), 0755))
			f, err = os.Create(filepath.Join(dir, "subDir", "bar"))
			require.NoError(t, err)
			require.NoError(t, f.Close())

			var localFilesIncludeFilterPrefix string
			if prefix == "emptyPrefix" {
				localFilesIncludeFilterPrefix = ""
			} else {
				localFilesIncludeFilterPrefix = prefix
			}
			s := s3put{
				AwsKey:                        "key",
				AwsSecret:                     "secret",
				Bucket:                        "bucket",
				BuildVariants:                 []string{},
				ContentType:                   "content-type",
				LocalFilesIncludeFilter:       []string{"*"},
				LocalFilesIncludeFilterPrefix: localFilesIncludeFilterPrefix,
				Permissions:                   s3.BucketCannedACLPublicRead,
				RemoteFile:                    "remote",
			}
			require.NoError(t, os.Mkdir(filepath.Join(dir, "destination"), 0755))
			opts := pail.LocalOptions{
				Path: filepath.Join(dir, "destination"),
			}
			s.bucket, err = pail.NewLocalBucket(opts)
			require.NoError(t, err)
			comm := client.NewMock("http://localhost.com")
			conf := &internal.TaskConfig{
				Expansions:   util.Expansions{},
				Task:         task.Task{Id: "mock_id", Secret: "mock_secret"},
				Project:      model.Project{},
				WorkDir:      dir,
				BuildVariant: model.BuildVariant{},
			}
			logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
			require.NoError(t, err)

			require.NoError(t, s.Execute(ctx, comm, logger, conf))
			it, err := s.bucket.List(ctx, "")
			require.NoError(t, err)
			expected := map[string]bool{
				"remotefoo": false,
				"remotebar": false,
			}
			for it.Next(ctx) {
				expected[it.Item().Name()] = true
			}
			require.Len(t, expected, 2)
			if localFilesIncludeFilterPrefix == "" {
				for item, exists := range expected {
					require.True(t, exists, item)
				}
			} else {
				require.True(t, expected["remotebar"])
				require.False(t, expected["remotefoo"])
			}
		})
	}
}

func TestFileUploadNaming(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subDir"), 0755))
	f, err := os.Create(filepath.Join(dir, "subDir", "bar"))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	s := s3put{
		AwsKey:                        "key",
		AwsSecret:                     "secret",
		Bucket:                        "bucket",
		BuildVariants:                 []string{},
		ContentType:                   "content-type",
		LocalFilesIncludeFilter:       []string{"*"},
		Permissions:                   s3.BucketCannedACLPublicRead,
		LocalFilesIncludeFilterPrefix: "",
		RemoteFile:                    "remote",
	}
	require.NoError(t, os.Mkdir(filepath.Join(dir, "destination"), 0755))
	opts := pail.LocalOptions{
		Path: filepath.Join(dir, "destination"),
	}
	s.bucket, err = pail.NewLocalBucket(opts)
	require.NoError(t, err)
	comm := client.NewMock("http://localhost.com")
	conf := &internal.TaskConfig{
		Expansions:   util.Expansions{},
		Task:         task.Task{Id: "mock_id", Secret: "mock_secret"},
		Project:      model.Project{},
		WorkDir:      dir,
		BuildVariant: model.BuildVariant{},
	}
	logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
	require.NoError(t, err)

	require.NoError(t, s.Execute(ctx, comm, logger, conf))
	attachedFiles := comm.AttachedFiles
	expected := map[string]bool{
		"remotebar":       false,
		"remoteremotebar": false,
	}

	for _, files := range attachedFiles {
		for _, f := range files {
			link := f.Link

			expected[filepath.Base(link)] = true
		}
	}

	require.True(t, expected["remotebar"])
	require.False(t, expected["remoteremotebar"])
}

func TestPreservePath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dir := t.TempDir()

	// Create the directories
	require.NoError(t, os.Mkdir(filepath.Join(dir, "myWebsite"), 0755))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "myWebsite", "assets"), 0755))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "myWebsite", "assets", "images"), 0755))

	// Create the files in in the assets directory
	f, err := os.Create(filepath.Join(dir, "foo"))
	require.NoError(t, err)
	require.NoError(t, f.Close())
	f, err = os.Create(filepath.Join(dir, "myWebsite", "assets", "asset1"))
	require.NoError(t, err)
	require.NoError(t, f.Close())
	f, err = os.Create(filepath.Join(dir, "myWebsite", "assets", "asset2"))
	require.NoError(t, err)
	require.NoError(t, f.Close())
	f, err = os.Create(filepath.Join(dir, "myWebsite", "assets", "asset3"))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// Create the files in the assets/images directory
	f, err = os.Create(filepath.Join(dir, "myWebsite", "assets", "images", "image1"))
	require.NoError(t, err)
	require.NoError(t, f.Close())
	f, err = os.Create(filepath.Join(dir, "myWebsite", "assets", "images", "image2"))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	s := s3put{
		AwsKey:                  "key",
		AwsSecret:               "secret",
		Bucket:                  "bucket",
		BuildVariants:           []string{},
		ContentType:             "content-type",
		LocalFilesIncludeFilter: []string{"*"},
		Permissions:             s3.BucketCannedACLPublicRead,
		RemoteFile:              "remote",
		PreservePath:            "true",
	}
	require.NoError(t, os.Mkdir(filepath.Join(dir, "destination"), 0755))
	opts := pail.LocalOptions{
		Path: filepath.Join(dir, "destination"),
	}
	s.bucket, err = pail.NewLocalBucket(opts)
	require.NoError(t, err)
	comm := client.NewMock("http://localhost.com")
	conf := &internal.TaskConfig{
		Expansions:   util.Expansions{},
		Task:         task.Task{Id: "mock_id", Secret: "mock_secret"},
		Project:      model.Project{},
		WorkDir:      dir,
		BuildVariant: model.BuildVariant{},
	}
	logger, err := comm.GetLoggerProducer(ctx, client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}, nil)
	require.NoError(t, err)

	require.NoError(t, s.Execute(ctx, comm, logger, conf))
	it, err := s.bucket.List(ctx, "")
	require.NoError(t, err)

	expected := map[string]bool{
		filepath.Join("remote", "foo"):                                     false,
		filepath.Join("remote", "myWebsite", "assets", "asset1"):           false,
		filepath.Join("remote", "myWebsite", "assets", "asset2"):           false,
		filepath.Join("remote", "myWebsite", "assets", "asset3"):           false,
		filepath.Join("remote", "myWebsite", "assets", "images", "image1"): false,
		filepath.Join("remote", "myWebsite", "assets", "images", "image2"): false,
	}

	for it.Next(ctx) {
		expected[it.Item().Name()] = true
	}

	for item, exists := range expected {
		require.True(t, exists, item)
	}

}
