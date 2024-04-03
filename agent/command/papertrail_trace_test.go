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
		"basic-no-expansions": {
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
		"basic-expansions": {
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

	ctx := context.Background()
	tempdir, err := os.MkdirTemp(os.TempDir(), "papertrail-test-*")
	if err != nil {
		t.Fatalf("make temp dir: %s", err)
	}

	t.Cleanup(func() { os.RemoveAll(tempdir) })

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

				if err := os.WriteFile(filename, []byte(data), 0755); err != nil {
					t.Fatalf("write temp file: %s", err)
				}

				filenames = append(filenames, filepath.Base(filename))

				h := sha256.New()
				if _, err := io.Copy(h, strings.NewReader(data)); err != nil {
					t.Fatalf("read file: %s", err)
				}

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

			if err := cmd.ParseParams(params); err != nil {
				t.Fatalf("parse params: %s", err)
			}

			tconf := &internal.TaskConfig{
				Expansions: util.Expansions(tc.expansions),
				Task: task.Task{
					Id:          tc.taskID,
					Execution:   tc.execution,
					ActivatedBy: tc.author,
				},
			}

			if err := cmd.Execute(ctx, nil, logger, tconf); err != nil {
				t.Fatalf("execute: %s", err)
			}

			product := getExpandedValue(tc.product)
			version := getExpandedValue(tc.version)

			pv, err := pclient.GetProductVersion(ctx, product, version)
			if err != nil {
				t.Fatalf("get product version: %s", err)
			}

			if len(pv.Spans) != len(filenames) {
				t.Fatalf("got %d spans, expected %d", len(pv.Spans), len(filenames))
			}

			for i, got := range pv.Spans {

				sum := checksums[i]
				filename := filenames[i]

				if got.SHA256sum != sum {
					t.Fatalf("got checksum '%s', expected '%s'", got.SHA256sum, sum)
				}

				if got.Filename != filename {
					t.Fatalf("got filename '%s', expected '%s'", got.Filename, filename)
				}

				build := getPapertrailBuildID(tc.taskID, tc.execution)

				if got.Build != build {
					t.Fatalf("got build '%s', expected '%s'", got.Build, build)
				}

				if got.Platform != "evergreen" {
					t.Fatalf("got platform '%s', expected 'evergreen'", got.Platform)
				}

				if got.Product != product {
					t.Fatalf("got product '%s', expected '%s'", got.Product, tc.product)
				}

				if got.Version != version {
					t.Fatalf("got version '%s', expected '%s'", got.Version, tc.version)
				}

				if got.Submitter != tc.author {
					t.Fatalf("got submitter '%s', expected '%s'", got.Submitter, tc.author)
				}

			}
		})
	}
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

		"missing-key-id": {
			keyID:     "",
			secretKey: "test-secret-key",
			product:   "test-product",
			version:   "test-version",
			filenames: []string{"test-filename"},
			err:       errors.New("must specify key_id"),
		},
		"missing-secret-key": {
			keyID:     "test-key",
			secretKey: "",
			product:   "test-product",
			version:   "test-version",
			filenames: []string{"test-filename"},
			err:       errors.New("must specify secret_key"),
		},
		"missing-product": {
			keyID:     "test-key",
			secretKey: "test-secret-key",
			product:   "",
			version:   "test-version",
			filenames: []string{"test-filename"},
			err:       errors.New("must specify product"),
		},
		"missing-version": {
			keyID:     "test-key",
			secretKey: "test-secret-key",
			product:   "test-product",
			version:   "",
			filenames: []string{"test-filename"},
			err:       errors.New("must specify version"),
		},
		"missing-filename": {
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
			if err.Error() != tc.err.Error() {
				t.Fatalf("expected error '%s', got '%s'", tc.err, err)
			}
		})
	}

	t.Run("no-errors", func(t *testing.T) {
		params := map[string]any{
			"key_id":     "test-key",
			"secret_key": "test-secret-key",
			"product":    "test-product",
			"version":    "test-version",
			"filenames":  "test-filename",
		}

		cmd := papertrailTraceFactory()

		err := cmd.ParseParams(params)
		if err != nil {
			t.Fatalf("expected no error, got '%s'", err)
		}
	})
}
