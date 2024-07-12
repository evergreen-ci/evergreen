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

	AddedImageEntryAction   = "ADDED"
	UpdatedImageEntryAction = "UPDATED"
	DeletedImageEntryAction = "DELETED"
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

// OSInfo stores operating system information.
type OSInfo struct {
	Version string
	Name    string
}

// getOSInfo returns a list of operating system information for an AMI.
func (c *RuntimeEnvironmentsClient) getOSInfo(ctx context.Context, amiID string, page, limit int) ([]OSInfo, error) {
	params := url.Values{}
	params.Set("ami", amiID)
	params.Set("page", strconv.Itoa(page))
	params.Set("limit", strconv.Itoa(limit))
	params.Set("type", OSType)
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
	amiHistory := []ImageHistoryInfo{}
	if err := gimlet.GetJSON(resp.Body, &amiHistory); err != nil {
		return nil, errors.Wrap(err, "decoding http body")
	}
	return amiHistory, nil
}

// ImageEventEntry represents a change to the image.
type ImageEventEntry struct {
	Name   string
	After  string
	Before string
	Type   string
	Action string
}

// ImageEvent contains information about changes to an image when the AMI changes.
type ImageEvent struct {
	Entries   []ImageEventEntry
	Timestamp time.Time
	AMIBefore string
	AMIAfter  string
}

// EventHistoryOptions represents the filtering arguments for getEvents. Image and Limit are required argument.
type EventHistoryOptions struct {
	Image string
	Page  int
	Limit int
}

// stringToTime converts a string representing time to type time.Time.
func stringToTime(timeInitial string) (time.Time, error) {
	timestamp, err := strconv.ParseInt(timeInitial, 10, 64)
	if err != nil {
		return time.Time{}, errors.Wrapf(err, "converting string '%s' to time", timeInitial)
	}
	return time.Unix(timestamp, 0), nil
}

// buildImageEventEntry make an ImageEventEntry given an ImageDiffChange.
func buildImageEventEntry(diff ImageDiffChange) (*ImageEventEntry, error) {
	action := ""
	if diff.Added != "" && diff.Removed != "" {
		action = UpdatedImageEntryAction
	} else if diff.Added != "" {
		action = AddedImageEntryAction
	} else if diff.Removed != "" {
		action = DeletedImageEntryAction
	} else {
		return nil, errors.New("neither added nor removed")
	}
	entry := ImageEventEntry{
		Name:   diff.Name,
		After:  diff.Added,
		Before: diff.Removed,
		Type:   diff.Type,
		Action: action,
	}
	return &entry, nil
}

// getEvents returns information about the changes between AMIs that occurred on the image.
func (c *RuntimeEnvironmentsClient) getEvents(ctx context.Context, opts EventHistoryOptions) ([]ImageEvent, error) {
	if opts.Limit == 0 {
		return nil, errors.New("no limit provided")
	}
	optsHistory := DistroHistoryFilterOptions{
		Distro: opts.Image,
		Page:   opts.Page,
		// Add 1 to ensure that the number of ImageEvents returned matches the limit.
		Limit: opts.Limit + 1,
	}
	imageHistory, err := c.getHistory(ctx, optsHistory)
	if err != nil {
		return nil, errors.Wrap(err, "getting image history")
	}
	result := []ImageEvent{}
	// Loop through the imageHistory which are in order from most recent to last to populate the
	// changes between the images. We set the current index i as the AfterAMI and base the timestamp
	// from the current index i.
	for i := 0; i < len(imageHistory)-1; i++ {
		amiBefore := imageHistory[i+1].AMI
		optsImageDiffs := ImageDiffOptions{
			BeforeAMI: amiBefore,
			AfterAMI:  imageHistory[i].AMI,
		}
		imageDiffs, err := c.getImageDiff(ctx, optsImageDiffs)
		if err != nil {
			return nil, errors.Wrap(err, "getting image differences")
		}
		entries := []ImageEventEntry{}
		for _, diff := range imageDiffs {
			entry, err := buildImageEventEntry(diff)
			if err != nil {
				return nil, errors.Wrap(err, "building image event entry")
			}
			entries = append(entries, *entry)
		}
		timestamp, err := stringToTime(imageHistory[i].CreationDate)
		if err != nil {
			return nil, errors.Wrap(err, "converting creation date")
		}
		imageEvent := ImageEvent{
			Entries:   entries,
			Timestamp: timestamp,
			AMIBefore: amiBefore,
			AMIAfter:  imageHistory[i].AMI,
		}
		result = append(result, imageEvent)
	}
	return result, nil
}
