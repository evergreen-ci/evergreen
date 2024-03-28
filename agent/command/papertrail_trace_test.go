package command

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentinternal "github.com/evergreen-ci/evergreen/agent/internal"
	internalclient "github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model/task"
	evergreenutil "github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	evergreenutility "github.com/evergreen-ci/utility"
	gripsend "github.com/mongodb/grip/send"
)

func TestPapertrailTrace(t *testing.T) {
	if os.Getenv("PAPERTRAIL_TEST") == "" {
		t.Skipf("set the PAPERTRAIL_TEST env var to '1' to run")
	}

	keyID := os.Getenv("PAPERTRAIL_KEY_ID")
	secretKey := os.Getenv("PAPERTRAIL_SECRET_KEY")
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
			version:   evergreenutility.RandomString(),
			filenames: []string{
				evergreenutility.RandomString(),
				evergreenutility.RandomString(),
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
				"version":     evergreenutility.RandomString(),
				"test-file-1": evergreenutility.RandomString(),
				"test-file-2": evergreenutility.RandomString(),
			},
		},
	}

	ctx := context.Background()
	tempdir, err := os.MkdirTemp(os.TempDir(), "papertrail-test-*")
	if err != nil {
		t.Fatalf("make temp dir: %s", err)
	}

	t.Cleanup(func() { os.RemoveAll(tempdir) })

	client := papertrailClient{
		httpc:     &http.Client{},
		address:   productionPapertrailAddress,
		keyID:     keyID,
		secretKey: secretKey,
	}

	sender := gripsend.MakeInternalLogger()
	logger := internalclient.NewSingleChannelLogHarness("test", sender)

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

			tconf := &agentinternal.TaskConfig{
				Expansions: evergreenutil.Expansions(tc.expansions),
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

			pv, err := client.getProductVersion(ctx, product, version)
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

				build := fmt.Sprintf("%s_%d", tc.taskID, tc.execution)

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
			err:       errMustSpecifyKeyID,
		},
		"missing-secret-key": {
			keyID:     "test-key",
			secretKey: "",
			product:   "test-product",
			version:   "test-version",
			filenames: []string{"test-filename"},
			err:       errMustSpecifySecretKey,
		},
		"missing-product": {
			keyID:     "test-key",
			secretKey: "test-secret-key",
			product:   "",
			version:   "test-version",
			filenames: []string{"test-filename"},
			err:       errMustSpecifyProduct,
		},
		"missing-version": {
			keyID:     "test-key",
			secretKey: "test-secret-key",
			product:   "test-product",
			version:   "",
			filenames: []string{"test-filename"},
			err:       errMustSpecifyVersion,
		},
		"missing-filename": {
			keyID:     "test-key",
			secretKey: "test-secret-key",
			product:   "test-product",
			version:   "test-version",
			filenames: []string{},
			err:       errMustSpecifyAtLeastOneFilename,
		},
		"no-errors": {
			keyID:     "test-key",
			secretKey: "test-secret-key",
			product:   "test-product",
			version:   "test-version",
			filenames: []string{"test-filename"},
			err:       nil,
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
			if !errors.Is(err, tc.err) {
				t.Fatalf("expected error '%s', got '%s'", tc.err, err)
			}
		})
	}
}

type papertrailSpan struct {
	ID        string `json:"id"`
	SHA256sum string `json:"sha256sum"`
	Filename  string `json:"filename"`
	Build     string `json:"build"`
	Platform  string `json:"platform"`
	Product   string `json:"product"`
	Version   string `json:"version"`
	Submitter string `json:"submitter"`
	KeyID     string `json:"key_id"`
	Time      uint64 `json:"time"`
}

type papertrailProductVersion struct {
	Product string           `json:"product"`
	Version string           `json:"version"`
	Spans   []papertrailSpan `json:"spans"`
}

func (c *papertrailClient) getProductVersion(ctx context.Context, product, version string) (*papertrailProductVersion, error) {
	params := url.Values{}

	params.Set("product", product)
	params.Set("version", version)
	params.Set("format", "json")

	addr := fmt.Sprintf("%s/product-version?%s", c.address, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr, nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}

	res, err := c.httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do post: %w", err)
	}

	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", res.StatusCode, string(b))
	}

	var v papertrailProductVersion

	if err := json.Unmarshal(b, &v); err != nil {
		return nil, fmt.Errorf("unmarshal: %w: %s", err, string(b))
	}

	return &v, nil
}
