package thirdparty

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
)

const (
	productionPapertrailAddress = "https://papertrail.prod.corp.mongodb.com"
)

// PapertrailSpan represents a span in json form from Papertrail.
type PapertrailSpan struct {
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

// PapertrailProductVersion contains a particular product-version's complete
// list of spans from Papertrail's /product-versions endpoint
type PapertrailProductVersion struct {
	Product string           `json:"product"`
	Version string           `json:"version"`
	Spans   []PapertrailSpan `json:"spans"`
}

// PapertrailClient is used to interact with the Papertrail service over HTTP.
// It is recommended to use NewPapertrailClient to initialize this struct.
type PapertrailClient struct {
	*http.Client
	Address   string
	KeyID     string
	SecretKey string
}

// NewPapertrailClient returns an initialized PapertrailClient. If address is
// empty, the default production address is used
func NewPapertrailClient(keyID, secretKey, address string) *PapertrailClient {
	if address == "" {
		address = productionPapertrailAddress
	}

	return &PapertrailClient{
		Client:    &http.Client{},
		Address:   address,
		KeyID:     keyID,
		SecretKey: secretKey,
	}
}

// TraceArgs contain the list of items necessary to pass to Papertrail
type TraceArgs struct {

	// Build is the unique identifier for a particular job run in a CI system.
	// For evergreen jobs, this is in the form "{task_id}_{execution}"
	Build string

	// Platform is the Papertrail-internal name for this CI service. It should
	// always be set to Evergreen unless doing testing
	Platform string

	// Filename is the basename of the file that is being traced
	Filename string

	// Sha256 is the hex-encoded checksum of the file that is being traced
	Sha256 string

	// Product is the product name to associate this file with (e.g. compass)
	Product string

	// Version is the version name to associate this file with (e.g. 1.0.0)
	Version string

	// Submitter is the username in the CI system to associate this trace with.
	Submitter string
}

// Trace calls the Papertrail /trace endpoint, associating a file and its
// metadata with a product version.
func (c *PapertrailClient) Trace(ctx context.Context, args TraceArgs) error {
	params := url.Values{}

	params.Set("build", args.Build)
	params.Set("platform", args.Platform)
	params.Set("filename", args.Filename)
	params.Set("sha256", args.Sha256)
	params.Set("product", args.Product)
	params.Set("version", args.Version)
	params.Set("submitter", args.Submitter)

	addr := fmt.Sprintf("%s/trace?%s", c.Address, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, addr, nil)
	if err != nil {
		return errors.Wrap(err, "getting new request")
	}

	req.Header.Set("X-PAPERTRAIL-KEY-ID", c.KeyID)
	req.Header.Set("X-PAPERTRAIL-SECRET-KEY", c.SecretKey)

	res, err := c.Do(req)
	if err != nil {
		return errors.Wrap(err, "calling do")
	}

	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return util.RespErrorf(res, "running papertrail trace")
	}

	return nil
}

// GetProductVersion calls the Papertrail /product-version endpoint, returning
// a PapertrailProductVersion with the collection of all spans associated with
// the product and version passed in.
func (c *PapertrailClient) GetProductVersion(ctx context.Context, product, version string) (*PapertrailProductVersion, error) {
	params := url.Values{}

	params.Set("product", product)
	params.Set("version", version)
	params.Set("format", "json")

	addr := fmt.Sprintf("%s/product-version?%s", c.Address, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr, nil)
	if err != nil {
		return nil, errors.Wrap(err, "getting new request")
	}

	res, err := c.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "calling do")
	}

	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading body")
	}

	if res.StatusCode != http.StatusOK {
		return nil, util.RespErrorf(res, "getting papertrail product version")
	}

	var v PapertrailProductVersion

	if err := json.Unmarshal(b, &v); err != nil {
		return nil, errors.Errorf("unmarshal: %s: %s", err, string(b))
	}

	return &v, nil
}
