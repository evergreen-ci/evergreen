package command

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestPapertrailTrace(t *testing.T) {
	if skip, _ := strconv.ParseBool(os.Getenv("SKIP_INTEGRATION_TESTS")); skip {
		t.Skip("SKIP_INTEGRATION_TESTS is set, skipping integration test")
	}

	settings := testutil.GetIntegrationFile(t)

	keyID := settings.Expansions["papertrail_key_id"]
	secretKey := settings.Expansions["papertrail_secret_key"]
	product := "papertrail-evergreen-command-testing"

	cases := map[string]struct {
		keyID      string
		secretKey  string
		product    string
		version    string
		filenames  []string
		author     string
		taskID     string
		execution  int
		expansions map[string]string
	}{
		"BasicNoExpansions": {
			keyID:     keyID,
			secretKey: secretKey,
			product:   product,
			version:   utility.RandomString(),
			filenames: []string{
				utility.RandomString(),
				utility.RandomString(),
			},
			author:    "foo.bar",
			taskID:    "abc123",
			execution: 1,
		},
		"BasicExpansions": {
			keyID:     "${key_id}",
			secretKey: "${secret_key}",
			product:   "${product}",
			version:   "${version}",
			filenames: []string{
				"${test-file-1}",
				"${test-file-2}",
			},
			author:    "foo.bar",
			taskID:    "abc123",
			execution: 1,
			expansions: map[string]string{
				"key_id":      keyID,
				"secret_key":  secretKey,
				"product":     product,
				"version":     utility.RandomString(),
				"test-file-1": utility.RandomString(),
				"test-file-2": utility.RandomString(),
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tempdir, err := os.MkdirTemp(os.TempDir(), "papertrail-trace-test-*")
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, os.RemoveAll(tempdir)) })

	pclient := thirdparty.NewPapertrailClient(keyID, secretKey, "")

	sender := send.MakeInternalLogger()
	logger := client.NewSingleChannelLogHarness("test", sender)

	for name, tc := range cases {
		name, tc := name, tc

		getExpandedValue := func(v string) string {
			if strings.HasPrefix(v, "$") {
				// trim '${}' from v
				v = v[2 : len(v)-1]
				return tc.expansions[v]
			}

			return v
		}

		t.Run(name, func(t *testing.T) {
			filenames := make([]string, 0, len(tc.filenames))
			checksums := make([]string, 0, len(tc.filenames))

			for _, basename := range tc.filenames {
				basename = getExpandedValue(basename)

				data := utility.RandomString()

				filename := filepath.Join(tempdir, basename)

				require.NoError(t, os.WriteFile(filename, []byte(data), 0755))

				filenames = append(filenames, filepath.Base(filename))

				h := sha256.New()
				_, err := io.Copy(h, strings.NewReader(data))
				require.NoError(t, err)

				sha256sum := fmt.Sprintf("%x", h.Sum(nil))

				checksums = append(checksums, sha256sum)
			}

			params := map[string]any{
				"key_id":     tc.keyID,
				"secret_key": tc.secretKey,
				"product":    tc.product,
				"version":    tc.version,
				"filenames":  filenames,
				"work_dir":   tempdir,
			}

			cmd := papertrailTraceFactory()

			require.NoError(t, cmd.ParseParams(params))

			tconf := &internal.TaskConfig{
				Expansions: util.Expansions(tc.expansions),
				Task: task.Task{
					Id:          tc.taskID,
					Execution:   tc.execution,
					ActivatedBy: tc.author,
				},
			}

			require.NoError(t, cmd.Execute(ctx, nil, logger, tconf))

			product := getExpandedValue(tc.product)
			version := getExpandedValue(tc.version)

			pv, err := pclient.GetProductVersion(ctx, product, version)
			require.NoError(t, err)

			require.Equal(t, len(pv.Spans), len(filenames))

			for i, got := range pv.Spans {

				sum := checksums[i]
				filename := filenames[i]

				require.Equal(t, sum, got.SHA256sum)
				require.Equal(t, filename, got.Filename)

				build := getPapertrailBuildID(tc.taskID, tc.execution)

				require.Equal(t, build, got.Build)
				require.Equal(t, "evergreen", got.Platform)
				require.Equal(t, product, got.Product)
				require.Equal(t, version, got.Version)
				require.Equal(t, tc.author, got.Submitter)
			}
		})
	}
}

func TestPapertrailGetFiles(t *testing.T) {
	type testFile struct {
		path    string
		content string
		want    papertrailTraceFile
	}

	cases := map[string]struct {
		files    []testFile
		patterns []string
	}{
		"Simple": {
			files: []testFile{
				{
					path:    "test.tar.gz",
					content: "test1",
					want: papertrailTraceFile{
						filename: "test.tar.gz",
						sha256:   "1b4f0e9851971998e732078544c96b36c3d01cedf7caa332359d6f1d83567014",
					},
				},
			},
			patterns: []string{"test.tar.gz"},
		},
		"NestedSimple": {
			files: []testFile{
				{
					path:    "parent/test.tar.gz",
					content: "test1",
					want: papertrailTraceFile{
						filename: "test.tar.gz",
						sha256:   "1b4f0e9851971998e732078544c96b36c3d01cedf7caa332359d6f1d83567014",
					},
				},
			},
			patterns: []string{"parent/test.tar.gz"},
		},
		"Wildcard": {
			files: []testFile{
				{
					path:    "test1.tar.gz",
					content: "foo",
					want: papertrailTraceFile{
						filename: "test1.tar.gz",
						sha256:   "2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
					},
				},
				{
					path:    "test2.tar.gz",
					content: "bar",
					want: papertrailTraceFile{
						filename: "test2.tar.gz",
						sha256:   "fcde2b2edba56bf408601fb721fe9b5c338d10ee429ea04fae5511b68fbf8fb9",
					},
				},
			},
			patterns: []string{"*.tar.gz"},
		},
		"WildcardParent": {
			files: []testFile{
				{
					path:    "parent/test1.tar.gz",
					content: "foo",
					want: papertrailTraceFile{
						filename: "test1.tar.gz",
						sha256:   "2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
					},
				},
				{
					path:    "parent/test2.tar.gz",
					content: "bar",
					want: papertrailTraceFile{
						filename: "test2.tar.gz",
						sha256:   "fcde2b2edba56bf408601fb721fe9b5c338d10ee429ea04fae5511b68fbf8fb9",
					},
				},
			},
			patterns: []string{"parent/*.tar.gz"},
		},
		"MultipleWildcardParent": {
			files: []testFile{
				{
					path:    "test-1/parent/test1.tar.gz",
					content: "foo",
					want: papertrailTraceFile{
						filename: "test1.tar.gz",
						sha256:   "2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
					},
				},
				{
					path:    "test-2/parent/test2.tar.gz",
					content: "bar",
					want: papertrailTraceFile{
						filename: "test2.tar.gz",
						sha256:   "fcde2b2edba56bf408601fb721fe9b5c338d10ee429ea04fae5511b68fbf8fb9",
					},
				},
			},
			patterns: []string{"*/parent/*.tar.gz"},
		},
	}

	for name, tc := range cases {
		name, tc := name, tc
		t.Run(name, func(t *testing.T) {
			tempdir, err := os.MkdirTemp(os.TempDir(), "papertrail-get-files-test-*")
			require.NoError(t, err)

			t.Cleanup(func() { require.NoError(t, os.RemoveAll(tempdir)) })

			for _, f := range tc.files {
				path := filepath.Join(tempdir, f.path)
				parent := filepath.Dir(path)

				require.NoError(t, os.MkdirAll(parent, 0755))

				require.NoError(t, os.WriteFile(path, []byte(f.content), 0644))
			}

			got, err := getTraceFiles(tempdir, tc.patterns)
			require.NoError(t, err)

			require.Equal(t, len(got), len(tc.files))

			for i, gotf := range got {
				tcf := tc.files[i]

				require.Equal(t, tcf.want, gotf)
			}
		})
	}

	t.Run("MultipleMatches", func(t *testing.T) {
		tempdir, err := os.MkdirTemp(os.TempDir(), "papertrail-get-files-test-*")
		require.NoError(t, err)

		t.Cleanup(func() { require.NoError(t, os.RemoveAll(tempdir)) })

		filename := "test.tar.gz"
		content := "test-content"

		path := filepath.Join(tempdir, filename)

		require.NoError(t, os.WriteFile(path, []byte(content), 0644))

		patterns := []string{
			"*.tar.gz",
			"*.gz",
		}

		_, err = getTraceFiles(tempdir, patterns)
		require.Error(t, err)

		want := "file 'test.tar.gz' matched multiple filename patterns ('*.gz' and '*.tar.gz'); papertrail.trace requires uploaded filenames to be unique regardless of their path, please provide only unique file names"

		require.Equal(t, want, err.Error())
	})
}

func TestPapertrailParseParams(t *testing.T) {
	cases := map[string]struct {
		keyID     string
		secretKey string
		product   string
		version   string
		filenames []string
		err       error
	}{

		"MissingKeyID": {
			keyID:     "",
			secretKey: "test-secret-key",
			product:   "test-product",
			version:   "test-version",
			filenames: []string{"test-filename"},
			err:       errors.New("must specify key_id"),
		},
		"MissingSecretKey": {
			keyID:     "test-key",
			secretKey: "",
			product:   "test-product",
			version:   "test-version",
			filenames: []string{"test-filename"},
			err:       errors.New("must specify secret_key"),
		},
		"MissingProduct": {
			keyID:     "test-key",
			secretKey: "test-secret-key",
			product:   "",
			version:   "test-version",
			filenames: []string{"test-filename"},
			err:       errors.New("must specify product"),
		},
		"MissingVersion": {
			keyID:     "test-key",
			secretKey: "test-secret-key",
			product:   "test-product",
			version:   "",
			filenames: []string{"test-filename"},
			err:       errors.New("must specify version"),
		},
		"MissingFilename": {
			keyID:     "test-key",
			secretKey: "test-secret-key",
			product:   "test-product",
			version:   "test-version",
			filenames: []string{},
			err:       errors.New("must specify at least one filename"),
		},
	}

	for name, tc := range cases {
		name, tc := name, tc
		t.Run(name, func(t *testing.T) {
			params := map[string]any{
				"key_id":     tc.keyID,
				"secret_key": tc.secretKey,
				"product":    tc.product,
				"version":    tc.version,
				"filenames":  tc.filenames,
			}

			cmd := papertrailTraceFactory()

			err := cmd.ParseParams(params)
			require.Equal(t, tc.err.Error(), err.Error())
		})
	}

	t.Run("NoErrors", func(t *testing.T) {
		params := map[string]any{
			"key_id":     "test-key",
			"secret_key": "test-secret-key",
			"product":    "test-product",
			"version":    "test-version",
			"filenames":  "test-filename",
		}

		cmd := papertrailTraceFactory()

		err := cmd.ParseParams(params)
		require.NoError(t, err)
	})
}
