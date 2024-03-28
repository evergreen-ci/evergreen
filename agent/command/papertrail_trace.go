package command

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	agentinternal "github.com/evergreen-ci/evergreen/agent/internal"
	agentinternalclient "github.com/evergreen-ci/evergreen/agent/internal/client"
	evergreenutil "github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
)

const (
	productionPapertrailAddress = "https://papertrail.devprod-infra.prod.corp.mongodb.com"
)

var (
	errMustSpecifyKeyID              = fmt.Errorf("must specify key_id")
	errMustSpecifySecretKey          = fmt.Errorf("must specify secret_key")
	errMustSpecifyProduct            = fmt.Errorf("must specify product")
	errMustSpecifyVersion            = fmt.Errorf("must specify version")
	errMustSpecifyAtLeastOneFilename = fmt.Errorf("must specify at least one filename")
)

type papertrailTrace struct {
	Address   string   `mapstructure:"address" plugin:"expand"`
	KeyID     string   `mapstructure:"key_id" plugin:"expand"`
	SecretKey string   `mapstructure:"secret_key" plugin:"expand"`
	Product   string   `mapstructure:"product" plugin:"expand"`
	Version   string   `mapstructure:"version" plugin:"expand"`
	Filenames []string `mapstructure:"filenames" plugin:"expand"`
	WorkDir   string   `mapstructure:"work_dir" plugin:"expand"`
	base
}

func (t *papertrailTrace) Execute(ctx context.Context,
	comm agentinternalclient.Communicator, logger agentinternalclient.LoggerProducer, conf *agentinternal.TaskConfig) error {
	if err := evergreenutil.ExpandValues(t, &conf.Expansions); err != nil {
		return fmt.Errorf("apply expansions: %w", err)
	}

	client := &papertrailClient{
		httpc:     &http.Client{},
		address:   t.Address,
		keyID:     t.KeyID,
		secretKey: t.SecretKey,
	}

	task := conf.Task

	papertrailBuildID := fmt.Sprintf("%s_%d", task.Id, task.Execution)

	workdir := t.WorkDir
	if workdir == "" {
		workdir = conf.WorkDir
	}

	for _, file := range t.Filenames {
		fullname := filepath.Join(workdir, file)

		sha256sum, err := getSHA256(fullname)
		if err != nil {
			return fmt.Errorf("get sha256: %w", err)
		}

		// should already be the basename, but just in case
		basename := filepath.Base(file)

		args := traceArgs{
			build:     papertrailBuildID,
			platform:  "evergreen",
			filename:  basename,
			sha256:    sha256sum,
			product:   t.Product,
			version:   t.Version,
			submitter: task.ActivatedBy,
		}

		if err := client.trace(ctx, args); err != nil {
			return fmt.Errorf("run trace: %w", err)
		}

		logger.Task().Infof("Successfully traced '%s' (sha256://%s)", fullname, sha256sum)
	}

	return nil
}

func getSHA256(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}

	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	sha256sum := fmt.Sprintf("%x", h.Sum(nil))

	return sha256sum, nil
}

func (t *papertrailTrace) ParseParams(params map[string]any) error {
	conf := &mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           t,
	}

	decoder, err := mapstructure.NewDecoder(conf)
	if err != nil {
		return fmt.Errorf("new decoder: %w", err)
	}

	if err := decoder.Decode(params); err != nil {
		return fmt.Errorf("decode params: %w", err)
	}

	if t.Address == "" {
		t.Address = productionPapertrailAddress
	}

	if t.KeyID == "" {
		return errMustSpecifyKeyID
	}

	if t.SecretKey == "" {
		return errMustSpecifySecretKey
	}

	if t.Product == "" {
		return errMustSpecifyProduct
	}

	if t.Version == "" {
		return errMustSpecifyVersion
	}

	if len(t.Filenames) == 0 {
		return errMustSpecifyAtLeastOneFilename
	}

	return nil
}
func (t *papertrailTrace) Name() string { return "papertrail.trace" }

func papertrailTraceFactory() Command { return &papertrailTrace{} }

type papertrailClient struct {
	httpc     *http.Client
	address   string
	keyID     string
	secretKey string
}

type traceArgs struct {
	build     string
	platform  string
	filename  string
	sha256    string
	product   string
	version   string
	submitter string
}

func (c *papertrailClient) trace(ctx context.Context, args traceArgs) error {
	params := url.Values{}

	params.Set("build", args.build)
	params.Set("platform", args.platform)
	params.Set("filename", args.filename)
	params.Set("sha256", args.sha256)
	params.Set("product", args.product)
	params.Set("version", args.version)
	params.Set("submitter", args.submitter)

	addr := fmt.Sprintf("%s/trace?%s", c.address, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, addr, nil)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}

	req.Header.Set("X-PAPERTRAIL-KEY-ID", c.keyID)
	req.Header.Set("X-PAPERTRAIL-SECRET-KEY", c.secretKey)

	res, err := c.httpc.Do(req)
	if err != nil {
		return fmt.Errorf("do post: %w", err)
	}

	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d: %s", res.StatusCode, string(b))
	}

	return nil
}
