package thirdparty

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

const (
	PackagesType   = "Packages"
	ToolchainsType = "Toolchains"
	OSType         = "OS"
)

type RuntimeEnvironmentsClient struct {
	Client  *http.Client
	BaseURL string
	APIKey  string
}

func NewRuntimeEnvironmentsClient(baseURL string, apiKey string) *RuntimeEnvironmentsClient {
	c := RuntimeEnvironmentsClient{
		Client:  &http.Client{},
		BaseURL: baseURL,
		APIKey:  apiKey,
	}
	return &c
}

// GetImageNames returns a list of strings containing the names of all images from the runtime environments API.
func (c *RuntimeEnvironmentsClient) GetImageNames(ctx context.Context) ([]string, error) {
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
	sort.Strings(filteredImages)
	return filteredImages, nil
}

// Package represents a package's information.
type Package struct {
	Name    string
	Version string
	Manager string
}

// PackageFilterOptions represents the filtering arguments, each of which is optional except the AMI.
type PackageFilterOptions struct {
	AMI     string
	Page    int
	Limit   int
	Name    string // Filter by the name of the package.
	Manager string // Filter by the package manager (ex. pip).
}

// getPackages returns a list of packages from the corresponding AMI and filters in opts.
func (c *RuntimeEnvironmentsClient) getPackages(ctx context.Context, opts PackageFilterOptions) ([]Package, error) {
	if opts.AMI == "" {
		return nil, errors.New("no AMI provided")
	}
	params := url.Values{}
	params.Set("ami", opts.AMI)
	params.Set("page", strconv.Itoa(opts.Page))
	if opts.Limit != 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}
	params.Set("name", opts.Name)
	params.Set("manager", opts.Manager)
	params.Set("type", PackagesType)
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

// OSInfoFilterOptions represents the filtering options for GetOSInfo. Each argument is optional except for the AMI field.
type OSInfoFilterOptions struct {
	AMI   string
	Name  string
	Page  int
	Limit int
}

// OSInfo stores operating system information.
type OSInfo struct {
	Version string `json:"version"`
	Name    string `json:"name"`
}

// GetOSInfo returns a list of operating system information for an AMI.
func (c *RuntimeEnvironmentsClient) GetOSInfo(ctx context.Context, opts OSInfoFilterOptions) ([]OSInfo, error) {
	if opts.AMI == "" {
		return nil, errors.New("no AMI provided")
	}
	params := url.Values{}
	params.Set("ami", opts.AMI)
	params.Set("page", strconv.Itoa(opts.Page))
	if opts.Limit != 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}
	params.Set("type", OSType)
	params.Set("name", opts.Name)
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
		if c.Type == PackagesType || c.Type == ToolchainsType {
			filteredChanges = append(filteredChanges, c)
		}
	}
	return filteredChanges, nil
}

// Toolchain represents a toolchain's information.
type Toolchain struct {
	Name    string
	Version string
	Manager string
}

// ToolchainFilterOptions represents the filtering arguments, each of which is optional except for the AMI.
type ToolchainFilterOptions struct {
	AMI     string
	Page    int
	Limit   int
	Name    string // Filter by the name of the toolchain (ex. golang).
	Version string // Filter by the version (ex. go1.8.7).
}

// getToolchains returns a list of toolchains from the AMI and filters in the ToolchainFilterOptions.
func (c *RuntimeEnvironmentsClient) getToolchains(ctx context.Context, opts ToolchainFilterOptions) ([]Toolchain, error) {
	if opts.AMI == "" {
		return nil, errors.New("no AMI provided")
	}
	params := url.Values{}
	params.Set("ami", opts.AMI)
	params.Set("page", strconv.Itoa(opts.Page))
	if opts.Limit != 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}
	params.Set("name", opts.Name)
	params.Set("version", opts.Version)
	params.Set("type", ToolchainsType)
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
	var toolchains []Toolchain
	if err := gimlet.GetJSON(resp.Body, &toolchains); err != nil {
		return nil, errors.Wrap(err, "decoding http body")
	}
	return toolchains, nil
}

// ImageHistoryInfo represents information about an image with its AMI and creation date.
type ImageHistoryInfo struct {
	AMI          string `json:"ami_id"`
	CreationDate string `json:"created_date"`
}

// DistoHistoryFilter represents the filtering arguments for getHistory. The Distro field is required and the other fields are optional.
type DistroHistoryFilterOptions struct {
	Distro string
	Page   int
	Limit  int
}

// GetHistory returns a list of images with their AMI and creation date corresponding to the provided distro in the order of most recently
// created.
func (c *RuntimeEnvironmentsClient) GetHistory(ctx context.Context, opts DistroHistoryFilterOptions) ([]ImageHistoryInfo, error) {
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
	amiHistory := []ImageHistoryInfo{}
	if err := gimlet.GetJSON(resp.Body, &amiHistory); err != nil {
		return nil, errors.Wrap(err, "decoding http body")
	}
	return amiHistory, nil
}

// stringToTime converts a string representing time to type time.Time.
func stringToTime(timeInitial string) (time.Time, error) {
	timestamp, err := strconv.ParseInt(timeInitial, 10, 64)
	if err != nil {
		return time.Time{}, errors.Wrapf(err, "converting time: '%s'", timeInitial)
	}
	return time.Unix(timestamp, 0), nil
}

// ImageInfo stores information about an image including its name, version_id, kernel, ami, and its last deployed time.
type ImageInfo struct {
	Name         string
	VersionID    string
	Kernel       string
	LastDeployed time.Time
	AMI          string
}

// GetDistroInfo returns information about a distro.
func (c *RuntimeEnvironmentsClient) GetDistroInfo(ctx context.Context, imageID string) (*ImageInfo, error) {
	optsHistory := DistroHistoryFilterOptions{
		Distro: imageID,
		Limit:  1,
	}
	resultHistory, err := c.GetHistory(ctx, optsHistory)
	if err != nil {
		return nil, errors.Wrapf(err, "getting history for distro '%s': '%s'", imageID, err.Error())
	}
	if len(resultHistory) == 0 {
		return nil, errors.Errorf("history for distro '%s' not found", imageID)
	}
	if resultHistory[0].AMI == "" {
		return nil, errors.Errorf("latest ami for distro '%s' not found", imageID)
	}
	ami := resultHistory[0].AMI
	if resultHistory[0].CreationDate == "" {
		return nil, errors.Errorf("creation time for distro '%s' not found", imageID)
	}
	timestamp, err := stringToTime(resultHistory[0].CreationDate)
	if err != nil {
		return nil, errors.Wrap(err, "converting creation time: '%s'")
	}

	optsOS := OSInfoFilterOptions{
		AMI:  ami,
		Name: "PRETTY_NAME",
	}
	resultOS, err := c.GetOSInfo(ctx, optsOS)
	if err != nil {
		return nil, errors.Wrap(err, "getting OS info")
	}
	name := ""
	for _, osInfo := range resultOS {
		if osInfo.Name == "PRETTY_NAME" {
			name = osInfo.Version
		}
	}
	if name == "" {
		return nil, errors.Errorf("OS information field not found for distro: '%s'", imageID)
	}

	optsOS = OSInfoFilterOptions{
		AMI:  ami,
		Name: "Kernel",
	}
	resultOS, err = c.GetOSInfo(ctx, optsOS)
	if err != nil {
		return nil, errors.Wrap(err, "getting OS info")
	}
	kernel := ""
	for _, osInfo := range resultOS {
		if osInfo.Name == "Kernel" {
			kernel = osInfo.Version
		}
	}
	if kernel == "" {
		return nil, errors.Errorf("OS information kernel field not found for distro: '%s'", imageID)
	}

	optsOS = OSInfoFilterOptions{
		AMI:  ami,
		Name: "VERSION_ID",
	}
	resultOS, err = c.GetOSInfo(ctx, optsOS)
	if err != nil {
		return nil, errors.Wrap(err, "getting OS info")
	}
	versionID := ""
	for _, osInfo := range resultOS {
		if osInfo.Name == "VERSION_ID" {
			versionID = osInfo.Version
		}
	}
	if versionID == "" {
		return nil, errors.Errorf("OS information version_id field not found")
	}
	image := ImageInfo{
		Name:         name,
		VersionID:    versionID,
		Kernel:       kernel,
		LastDeployed: timestamp,
		AMI:          ami,
	}
	return &image, nil
}
