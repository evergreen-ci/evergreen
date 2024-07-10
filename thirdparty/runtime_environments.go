package thirdparty

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

const (
	EnvChangeTypePackages   = "Packages"
	EnvChangeTypeToolchains = "Toolchains"
)

type RuntimeEnvironmentsClient struct {
	Client  *http.Client
	BaseURL string
	APIKey  string
}

// OSInfo stores operating system information.
type OSInfo struct {
	Version string
	Name    string
}

func NewRuntimeEnvironmentsClient(baseURL string, apiKey string) *RuntimeEnvironmentsClient {
	c := RuntimeEnvironmentsClient{
		Client:  &http.Client{},
		BaseURL: baseURL,
		APIKey:  apiKey,
	}
	return &c
}

// getImageNames returns a list of strings containing the names of all images from the runtime environments API.
func (c *RuntimeEnvironmentsClient) getImageNames(ctx context.Context) ([]string, error) {
	apiURL := fmt.Sprintf("%s/rest/api/v1/imageList", c.BaseURL)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Api-Key", c.APIKey)
	resp, err := c.Client.Do(request)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("HTTP request returned unexpected status '%s': %s", resp.Status, string(msg))
	}
	var images []string
	if err := gimlet.GetJSON(resp.Body, &images); err != nil {
		return nil, errors.Wrap(err, "decoding http body")
	}
	if len(images) == 0 {
		return nil, errors.New("No corresponding images")
	}
	filteredImages := []string{}
	for _, img := range images {
		if img != "" {
			filteredImages = append(filteredImages, img)
		}
	}
	return filteredImages, nil
}

// Package represents a package's information.
type Package struct {
	Name    string
	Version string
	Manager string
}

// PackageFilterOptions represents the filtering arguments, each of which is optional except the AMIID.
type PackageFilterOptions struct {
	AMIID   string
	Page    int
	Limit   int
	Name    string // Filter by the name of the package.
	Manager string // Filter by the package manager (ex. pip).
}

// getPackages returns a list of packages from the corresponding AMI and filters in opts.
func (c *RuntimeEnvironmentsClient) getPackages(ctx context.Context, opts PackageFilterOptions) ([]Package, error) {
	params := url.Values{}
	if opts.AMIID == "" {
		return nil, errors.New("no AMI provided")
	}
	params.Set("ami", opts.AMIID)
	params.Set("page", strconv.Itoa(opts.Page))
	if opts.Limit != 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}
	params.Set("name", opts.Name)
	params.Set("manager", opts.Manager)
	params.Set("type", "Packages")
	apiURL := fmt.Sprintf("%s/rest/api/v1/image?%s", c.BaseURL, params.Encode())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Api-Key", c.APIKey)
	resp, err := c.Client.Do(request)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("HTTP request returned unexpected status '%s': %s", resp.Status, string(msg))
	}
	packages := []Package{}
	if err := gimlet.GetJSON(resp.Body, &packages); err != nil {
		return nil, errors.Wrap(err, "decoding http body")
	}
	return packages, nil
}

// getOSInfo returns a list of operating system information for an AMI.
func (c *RuntimeEnvironmentsClient) getOSInfo(ctx context.Context, amiID string, page, limit int) ([]OSInfo, error) {
	params := url.Values{}
	params.Set("ami", amiID)
	params.Set("page", strconv.Itoa(page))
	params.Set("limit", strconv.Itoa(limit))
	params.Set("type", "OS")
	apiURL := fmt.Sprintf("%s/rest/api/v1/image?%s", c.BaseURL, params.Encode())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Api-Key", c.APIKey)
	resp, err := c.Client.Do(request)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("HTTP request returned unexpected status '%s': %s", resp.Status, string(msg))
	}
	osInfo := []OSInfo{}
	if err := gimlet.GetJSON(resp.Body, &osInfo); err != nil {
		return nil, errors.Wrap(err, "decoding http body")
	}
	return osInfo, nil
}

// ImageDiffOptions represents the arguments for getImageDiff. BeforeAMI is the starting AMI, and AfterAMI is the ending AMI.
type ImageDiffOptions struct {
	BeforeAMI string
	AfterAMI  string
}

// ImageDiffChange represents a change between two AMIs.
type ImageDiffChange struct {
	Name    string
	Manager string
	Type    string
	Removed string
	Added   string
}

// getImageDiff returns a list of package and toolchain changes that occurred between the provided AMIs.
func (c *RuntimeEnvironmentsClient) getImageDiff(ctx context.Context, opts ImageDiffOptions) ([]ImageDiffChange, error) {
	params := url.Values{}
	params.Set("ami", opts.BeforeAMI)
	params.Set("ami2", opts.AfterAMI)
	params.Set("limit", "1000000000") // Artificial limit set high because API has default limit of 10.
	apiURL := fmt.Sprintf("%s/rest/api/v1/imageDiffs?%s", c.BaseURL, params.Encode())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Api-Key", c.APIKey)
	resp, err := c.Client.Do(request)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("HTTP request returned unexpected status '%s': %s", resp.Status, string(msg))
	}
	changes := []ImageDiffChange{}
	if err := gimlet.GetJSON(resp.Body, &changes); err != nil {
		return nil, errors.Wrap(err, "decoding http body")
	}
	filteredChanges := []ImageDiffChange{}
	for _, c := range changes {
		if c.Type == EnvChangeTypePackages || c.Type == EnvChangeTypeToolchains {
			filteredChanges = append(filteredChanges, c)
		}
	}
	return filteredChanges, nil
}

// ImageHistoryInfo represents information about an image with its AMI and creation date.
type ImageHistoryInfo struct {
	AMI          string
	CreationDate time.Time
}

// DistoHistoryFilter represents the filtering arguments for getHistory. The Distro field is required and the other fields are optional.
type DistroHistoryFilterOptions struct {
	Distro string
	Page   int
	Limit  int
}

// ImageHistoryInfoReceiver represents information about an image
type ImageHistoryInfoReceiver struct {
	AMI_ID       string
	Created_Date string
}

// getHistory returns a list of images with their AMI and creation date corresponding to the provided distro in the order of most recently
// created.
func (c *RuntimeEnvironmentsClient) getHistory(ctx context.Context, opts DistroHistoryFilterOptions) ([]ImageHistoryInfo, error) {
	if opts.Distro == "" {
		return nil, errors.New("no distro provided")
	}
	params := url.Values{}
	params.Set("distro", opts.Distro)
	params.Set("page", strconv.Itoa(opts.Page))
	if opts.Limit != 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}
	apiURL := fmt.Sprintf("%s/rest/api/v1/distroHistory?%s", c.BaseURL, params.Encode())
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Add("Content-Type", "application/json")
	request.Header.Add("Api-Key", c.APIKey)
	resp, err := c.Client.Do(request)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return nil, errors.Errorf("HTTP request returned unexpected status '%s': %s", resp.Status, string(msg))
	}
	amiHistory := []ImageHistoryInfoReceiver{}
	if err := gimlet.GetJSON(resp.Body, &amiHistory); err != nil {
		return nil, errors.Wrap(err, "decoding http body")
	}
	amiHistoryParsed := []ImageHistoryInfo{}
	for _, img := range amiHistory {
		if img.AMI_ID == "" {
			return nil, errors.New("no AMI found")
		}
		if img.Created_Date == "" {
			return nil, errors.New("no creation found")
		}
		timestamp, err := strconv.ParseInt(img.Created_Date, 10, 64)
		if err != nil {
			return nil, errors.Wrap(err, "converting creation date")
		}
		image := ImageHistoryInfo{
			AMI:          img.AMI_ID,
			CreationDate: time.Unix(timestamp, 0),
		}
		amiHistoryParsed = append(amiHistoryParsed, image)
	}
	return amiHistoryParsed, nil
}
