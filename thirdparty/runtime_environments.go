package thirdparty

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type ImageEventEntryAction string

const (
	runtimeEnvironmentsAPIAlert = "Runtime Environments API alerting"
)

const (
	ImageEventEntryActionAdded   ImageEventEntryAction = "ADDED"
	ImageEventEntryActionUpdated ImageEventEntryAction = "UPDATED"
	ImageEventEntryActionDeleted ImageEventEntryAction = "DELETED"
)

type ImageEventType string

const (
	ImageEventTypeOperatingSystem ImageEventType = "OPERATING_SYSTEM"
	ImageEventTypePackage         ImageEventType = "PACKAGE"
	ImageEventTypeToolchain       ImageEventType = "TOOLCHAIN"
	ImageEventTypeFile            ImageEventType = "FILE"
)

const (
	APITypeOS         = "OS"
	APITypePackages   = "Packages"
	APITypeToolchains = "Toolchains"
	APITypeFiles      = "Files"
)

// RuntimeEnvironmentsClient is a client that can communicate with the Runtime Environments API.
// Interacting with the API requires an API key.
type RuntimeEnvironmentsClient struct {
	Client  *http.Client
	BaseURL string
	APIKey  string
}

// NewRuntimeEnvironmentsClient returns a client that can interact with the Runtime Environments API.
func NewRuntimeEnvironmentsClient(baseURL string, apiKey string) *RuntimeEnvironmentsClient {
	c := RuntimeEnvironmentsClient{
		Client:  &http.Client{},
		BaseURL: baseURL,
		APIKey:  apiKey,
	}
	return &c
}

// DoRequest makes a request to the image visibility API.
func (c *RuntimeEnvironmentsClient) DoRequest(ctx context.Context, route string, encodedParams string) ([]byte, error) {
	apiURL := fmt.Sprintf("%s/rest/api/v1/ami%s", c.BaseURL, route)
	if len(encodedParams) > 0 {
		apiURL += "?" + encodedParams
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Add(evergreen.ContentTypeHeader, evergreen.ContentTypeValue)
	request.Header.Add(evergreen.APIKeyHeader, c.APIKey)
	resp, err := c.Client.Do(request)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		apiErr := errors.New(string(msg))
		grip.Debug(message.WrapError(apiErr, message.Fields{
			"message":     "bad response code from image visibility API",
			"reason":      runtimeEnvironmentsAPIAlert,
			"status_code": resp.StatusCode,
		}))
		return nil, apiErr
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading bytes from response body")
	}
	return body, nil
}

// GetImageNames returns a list of strings containing the names of all images from the Runtime Environments API.
func (c *RuntimeEnvironmentsClient) GetImageNames(ctx context.Context) ([]string, error) {
	body, err := c.DoRequest(ctx, "/list", "")
	if err != nil {
		return nil, errors.Wrap(err, "executing request to API")
	}

	var images []string
	if err = json.Unmarshal(body, &images); err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"message": "parsing response from image visibility API",
			"reason":  runtimeEnvironmentsAPIAlert,
		}))
		return nil, errors.Wrap(err, "unmarshalling request body")
	}
	if len(images) == 0 {
		return nil, errors.New("no corresponding images")
	}
	return images, nil
}

// OSInfoResponse represents a response from the /rest/api/v1/ami/os route.
type OSInfoResponse struct {
	Data          []OSInfo `json:"data"`
	FilteredCount int      `json:"filtered_count"`
	TotalCount    int      `json:"total_count"`
}

// OSInfo stores operating system information.
type OSInfo struct {
	Name string `json:"name"`
	// Value associated with the OS field.
	Version string `json:"version"`
}

// OSInfoFilterOptions represents the filtering options for GetOSInfo. Each argument is optional except for the AMI field.
type OSInfoFilterOptions struct {
	AMI   string `json:"-"`
	Name  string `json:"-"`
	Page  int    `json:"-"`
	Limit int    `json:"-"`
}

// GetOSInfo returns a list of operating system information for an AMI.
func (c *RuntimeEnvironmentsClient) GetOSInfo(ctx context.Context, opts OSInfoFilterOptions) (*OSInfoResponse, error) {
	if opts.AMI == "" {
		return nil, errors.New("no AMI provided")
	}
	params := url.Values{}
	params.Set("id", opts.AMI)
	params.Set("page", strconv.Itoa(opts.Page))
	if opts.Limit != 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}
	params.Set("data_name", opts.Name)

	body, err := c.DoRequest(ctx, "/os", params.Encode())
	if err != nil {
		return nil, errors.Wrap(err, "executing request to API")
	}

	osInfo := &OSInfoResponse{}
	if err = json.Unmarshal(body, &osInfo); err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"message": "parsing response from image visibility API",
			"reason":  runtimeEnvironmentsAPIAlert,
		}))
		return nil, errors.Wrap(err, "unmarshalling request body")
	}
	return osInfo, nil
}

// APIPackageResponse represents a response from the /rest/api/v1/ami/packages route.
type APIPackageResponse struct {
	Data          []Package `json:"data"`
	FilteredCount int       `json:"filtered_count"`
	TotalCount    int       `json:"total_count"`
}

// Package represents a package's information.
type Package struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	// Manager of the package (e.g. pip).
	Manager string `json:"manager"`
}

// PackageFilterOptions represents the filtering arguments, each of which is optional except the AMI.
type PackageFilterOptions struct {
	AMI     string `json:"-"`
	Page    int    `json:"-"`
	Limit   int    `json:"-"`
	Name    string `json:"-"` // Filter by the name of the package.
	Manager string `json:"-"` // Filter by the package manager (ex. pip).
}

// GetPackages returns a list of packages from the corresponding AMI and filters in opts.
func (c *RuntimeEnvironmentsClient) GetPackages(ctx context.Context, opts PackageFilterOptions) (*APIPackageResponse, error) {
	if opts.AMI == "" {
		return nil, errors.New("no AMI provided")
	}
	params := url.Values{}
	params.Set("id", opts.AMI)
	params.Set("page", strconv.Itoa(opts.Page))
	if opts.Limit != 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}
	params.Set("data_name", opts.Name)

	body, err := c.DoRequest(ctx, "/packages", params.Encode())
	if err != nil {
		return nil, errors.Wrap(err, "executing request to API")
	}

	packages := &APIPackageResponse{}
	if err = json.Unmarshal(body, &packages); err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"message": "parsing response from image visibility API",
			"reason":  runtimeEnvironmentsAPIAlert,
		}))
		return nil, errors.Wrap(err, "unmarshalling request body")
	}
	return packages, nil
}

// APIToolchainResponse represents a response from the /rest/api/v1/ami/toolchains route.
type APIToolchainResponse struct {
	Data          []Toolchain `json:"data"`
	FilteredCount int         `json:"filtered_count"`
	TotalCount    int         `json:"total_count"`
}

// Toolchain represents a toolchain's information.
type Toolchain struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	// Path to the toolchain.
	Manager string `json:"manager"`
}

// ToolchainFilterOptions represents the filtering arguments, each of which is optional except for the AMI.
type ToolchainFilterOptions struct {
	AMI     string `json:"-"`
	Page    int    `json:"-"`
	Limit   int    `json:"-"`
	Name    string `json:"-"` // Filter by the name of the toolchain (ex. golang).
	Version string `json:"-"` // Filter by the version (ex. go1.8.7).
}

// GetToolchains returns a list of toolchains from the AMI and filters in the ToolchainFilterOptions.
func (c *RuntimeEnvironmentsClient) GetToolchains(ctx context.Context, opts ToolchainFilterOptions) (*APIToolchainResponse, error) {
	if opts.AMI == "" {
		return nil, errors.New("no AMI provided")
	}
	params := url.Values{}
	params.Set("id", opts.AMI)
	params.Set("page", strconv.Itoa(opts.Page))
	if opts.Limit != 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}
	params.Set("data_name", opts.Name)

	body, err := c.DoRequest(ctx, "/toolchains", params.Encode())
	if err != nil {
		return nil, errors.Wrap(err, "executing request to API")
	}

	toolchains := &APIToolchainResponse{}
	if err = json.Unmarshal(body, &toolchains); err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"message": "parsing response from image visibility API",
			"reason":  runtimeEnvironmentsAPIAlert,
		}))
		return nil, errors.Wrap(err, "unmarshalling request body")
	}
	return toolchains, nil
}

// APIFileResponse represents a response from the /rest/api/v1/ami/files route.
type APIFileResponse struct {
	Data          []File `json:"data"`
	FilteredCount int    `json:"filtered_count"`
	TotalCount    int    `json:"total_count"`
}

// File represents a file's information.
type File struct {
	Name string `json:"name"`
	// File SHA-256.
	Version string `json:"version"`
	// Path to the file.
	Manager string `json:"manager"`
}

// FileFilterOptions represents the filtering arguments, each of which is optional except for the AMI.
type FileFilterOptions struct {
	AMI   string `json:"-"`
	Page  int    `json:"-"`
	Limit int    `json:"-"`
	Name  string `json:"-"` // Filter by the name of the file (ex. something.pem).
}

// GetFiles returns a list of files from the AMI and filters in the FileFilterOptions.
func (c *RuntimeEnvironmentsClient) GetFiles(ctx context.Context, opts FileFilterOptions) (*APIFileResponse, error) {
	if opts.AMI == "" {
		return nil, errors.New("no AMI provided")
	}
	params := url.Values{}
	params.Set("id", opts.AMI)
	params.Set("page", strconv.Itoa(opts.Page))
	if opts.Limit != 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}
	params.Set("data_name", opts.Name)

	body, err := c.DoRequest(ctx, "/files", params.Encode())
	if err != nil {
		return nil, errors.Wrap(err, "executing request to API")
	}

	files := &APIFileResponse{}
	if err = json.Unmarshal(body, &files); err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"message": "parsing response from image visibility API",
			"reason":  runtimeEnvironmentsAPIAlert,
		}))
		return nil, errors.Wrap(err, "unmarshalling request body")
	}
	return files, nil
}

// APIDiffResponse represents a response from the /rest/api/v1/ami/diff route.
type APIDiffResponse struct {
	Data          []ImageDiffChange `json:"data"`
	FilteredCount int               `json:"filtered_count"`
	TotalCount    int               `json:"total_count"`
}

// ImageDiffChange represents a change between two AMIs.
type ImageDiffChange struct {
	AfterVersion  string `json:"after_version"`
	BeforeVersion string `json:"before_version"`
	Name          string `json:"name"`
	Manager       string `json:"manager"`
	Type          string `json:"type"`
}

// diffFilterOptions represents the arguments for getImageDiff. AMIBefore is the starting AMI, and AMIAfter is the ending AMI.
type diffFilterOptions struct {
	AMIBefore string `json:"-"`
	AMIAfter  string `json:"-"`
}

// getImageDiff returns a list of package and toolchain changes that occurred between the provided AMIs.
func (c *RuntimeEnvironmentsClient) getImageDiff(ctx context.Context, opts diffFilterOptions) ([]ImageDiffChange, error) {
	params := url.Values{}
	params.Set("before_id", opts.AMIBefore)
	params.Set("after_id", opts.AMIAfter)
	params.Set("limit", "1000000000") // Artificial limit set high because API has default limit of 10.

	body, err := c.DoRequest(ctx, "/diff", params.Encode())
	if err != nil {
		return nil, errors.Wrap(err, "executing request to API")
	}

	changes := &APIDiffResponse{}
	if err = json.Unmarshal(body, &changes); err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"message": "parsing response from image visibility API",
			"reason":  runtimeEnvironmentsAPIAlert,
		}))
		return nil, errors.Wrap(err, "unmarshalling request body")
	}
	filteredChanges := []ImageDiffChange{}
	for _, c := range changes.Data {
		if c.Type == APITypeOS || c.Type == APITypePackages || c.Type == APITypeToolchains || c.Type == APITypeFiles {
			filteredChanges = append(filteredChanges, c)
		}
	}
	return filteredChanges, nil
}

// APIHistoryResponse represents a response from the /rest/api/v1/ami/history route.
type APIHistoryResponse struct {
	Data       []ImageHistoryInfo `json:"data"`
	TotalCount int                `json:"total_count"`
}

// ImageHistoryInfo represents information about an image with its AMI and creation date.
type ImageHistoryInfo struct {
	AMI          string `json:"ami_id"`
	CreationDate int    `json:"created_date"`
}

// historyFilterOptions represents the filtering arguments for getHistory. The ImageID field is required and the other fields are optional.
type historyFilterOptions struct {
	ImageID string `json:"-"`
	Page    int    `json:"-"`
	Limit   int    `json:"-"`
}

// getHistory returns a list of images with their AMI and creation date corresponding to the provided distro in the order of most recently
// created.
func (c *RuntimeEnvironmentsClient) getHistory(ctx context.Context, opts historyFilterOptions) ([]ImageHistoryInfo, error) {
	if opts.ImageID == "" {
		return nil, errors.New("no image provided")
	}
	params := url.Values{}
	params.Set("name", opts.ImageID)
	params.Set("page", strconv.Itoa(opts.Page))
	if opts.Limit != 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}

	body, err := c.DoRequest(ctx, "/history", params.Encode())
	if err != nil {
		return nil, errors.Wrap(err, "executing request to API")
	}

	amiHistory := &APIHistoryResponse{}
	if err = json.Unmarshal(body, &amiHistory); err != nil {
		grip.Debug(message.WrapError(err, message.Fields{
			"message": "parsing response from image visibility API",
			"reason":  runtimeEnvironmentsAPIAlert,
		}))
		return nil, errors.Wrap(err, "unmarshalling request body")
	}
	return amiHistory.Data, nil
}

// ImageEventEntry represents a change to the image.
type ImageEventEntry struct {
	Name   string
	Before string
	After  string
	Type   ImageEventType
	Action ImageEventEntryAction
}

// ImageEvent contains information about changes to an image when the AMI changes.
type ImageEvent struct {
	Entries   []ImageEventEntry
	Timestamp time.Time
	AMIBefore string
	AMIAfter  string
}

// EventHistoryOptions represents the filtering arguments for GetEvents. Image and Limit are required arguments.
type EventHistoryOptions struct {
	Image string `json:"-"`
	Page  int    `json:"-"`
	Limit int    `json:"-"`
}

// GetEvents returns information about the changes between AMIs that occurred on the image.
func (c *RuntimeEnvironmentsClient) GetEvents(ctx context.Context, opts EventHistoryOptions) ([]ImageEvent, error) {
	if opts.Limit == 0 {
		return nil, errors.New("no limit provided")
	}
	optsHistory := historyFilterOptions{
		ImageID: opts.Image,
		Page:    opts.Page,
		// Diffing two AMIs only produces one ImageEvent. We need to add 1 so that the number of returned events is equal to the limit.
		Limit: opts.Limit + 1,
	}
	imageHistory, err := c.getHistory(ctx, optsHistory)
	if err != nil {
		return nil, errors.Wrap(err, "getting image history")
	}
	result := []ImageEvent{}
	// Loop through the imageHistory which are in order from most recent to last to populate the
	// changes between the images. We set the current index i as the AMIAfter and base the timestamp
	// from the current index i.
	for i := 0; i < len(imageHistory)-1; i++ {
		amiBefore := imageHistory[i+1].AMI
		amiAfter := imageHistory[i].AMI
		optsImageDiffs := diffFilterOptions{
			AMIBefore: amiBefore,
			AMIAfter:  amiAfter,
		}
		imageDiffs, err := c.getImageDiff(ctx, optsImageDiffs)
		if err != nil {
			return nil, errors.Wrap(err, fmt.Sprintf("getting image diffs between AMIs '%s' and '%s'", amiBefore, amiAfter))
		}
		entries := []ImageEventEntry{}
		for _, diff := range imageDiffs {
			entry, err := buildImageEventEntry(diff)
			if err != nil {
				return nil, errors.Wrap(err, "building image event entry")
			}
			entries = append(entries, *entry)
		}
		imageEvent := ImageEvent{
			Entries:   entries,
			Timestamp: intToTime(imageHistory[i].CreationDate),
			AMIBefore: amiBefore,
			AMIAfter:  amiAfter,
		}
		result = append(result, imageEvent)
	}
	return result, nil
}

// DistroImage stores information about an image including its AMI, ID, and last deployed time. An example of an
// image ID would be "ubuntu2204", which would correspond to the distros "ubuntu2204-small" and "ubuntu2204-large".
type DistroImage struct {
	ID           string
	AMI          string
	LastDeployed time.Time
}

// GetImageInfo returns basic information about an image.
func (c *RuntimeEnvironmentsClient) GetImageInfo(ctx context.Context, imageID string) (*DistroImage, error) {
	historyOpts := historyFilterOptions{
		ImageID: imageID,
		Limit:   1,
	}
	amiHistory, err := c.getHistory(ctx, historyOpts)
	if err != nil {
		return nil, errors.Wrapf(err, "getting latest AMI and timestamp")
	}
	if len(amiHistory) != 1 {
		grip.Debug(message.Fields{
			"message":  "expected exactly 1 history result for image",
			"reason":   runtimeEnvironmentsAPIAlert,
			"image_id": imageID,
		})
		return nil, errors.Errorf("expected exactly 1 history result for image '%s'", imageID)
	}
	return &DistroImage{
		ID:           imageID,
		AMI:          amiHistory[0].AMI,
		LastDeployed: intToTime(amiHistory[0].CreationDate),
	}, nil
}

// buildImageEventEntry make an ImageEventEntry given an ImageDiffChange.
func buildImageEventEntry(diff ImageDiffChange) (*ImageEventEntry, error) {
	var eventAction ImageEventEntryAction
	if diff.AfterVersion != "" && diff.BeforeVersion != "" {
		eventAction = ImageEventEntryActionUpdated
	} else if diff.AfterVersion != "" {
		eventAction = ImageEventEntryActionAdded
	} else if diff.BeforeVersion != "" {
		eventAction = ImageEventEntryActionDeleted
	} else {
		return nil, errors.New(fmt.Sprintf("item '%s' was neither added nor removed", diff.Name))
	}

	var eventType ImageEventType
	switch diff.Type {
	case APITypeOS:
		eventType = ImageEventTypeOperatingSystem
	case APITypePackages:
		eventType = ImageEventTypePackage
	case APITypeToolchains:
		eventType = ImageEventTypeToolchain
	case APITypeFiles:
		eventType = ImageEventTypeFile
	default:
		return nil, errors.New(fmt.Sprintf("item '%s' has unrecognized event type '%s'", diff.Name, diff.Type))
	}

	entry := ImageEventEntry{
		Name:   diff.Name,
		After:  diff.AfterVersion,
		Before: diff.BeforeVersion,
		Type:   eventType,
		Action: eventAction,
	}
	return &entry, nil
}

// intToTime converts a int representing a timestamp to time.Time.
func intToTime(timeInitial int) time.Time {
	return time.Unix(int64(timeInitial), 0)
}
