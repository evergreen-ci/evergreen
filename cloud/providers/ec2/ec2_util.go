package ec2

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/user"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	gcec2 "github.com/dynport/gocloud/aws/ec2"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db/bsonutil"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/goamz/goamz/aws"
	"github.com/goamz/goamz/ec2"
	"github.com/tychoish/grip"
)

const (
	OnDemandProviderName = "ec2"
	SpotProviderName     = "ec2-spot"
	NameTimeFormat       = "20060102150405"
	SpawnHostExpireDays  = 90
	MciHostExpireDays    = 30
)

type MountPoint struct {
	VirtualName string `mapstructure:"virtual_name" json:"virtual_name,omitempty" bson:"virtual_name,omitempty"`
	DeviceName  string `mapstructure:"device_name" json:"device_name,omitempty" bson:"device_name,omitempty"`
	Size        int    `mapstructure:"size" json:"size,omitempty" bson:"size,omitempty"`
}

var (
	// bson fields for the EC2ProviderSettings struct
	AMIKey           = bsonutil.MustHaveTag(EC2ProviderSettings{}, "AMI")
	InstanceTypeKey  = bsonutil.MustHaveTag(EC2ProviderSettings{}, "InstanceType")
	SecurityGroupKey = bsonutil.MustHaveTag(EC2ProviderSettings{}, "SecurityGroup")
	KeyNameKey       = bsonutil.MustHaveTag(EC2ProviderSettings{}, "KeyName")
	MountPointsKey   = bsonutil.MustHaveTag(EC2ProviderSettings{}, "MountPoints")
)

var (
	// bson fields for the EC2SpotSettings struct
	BidPriceKey = bsonutil.MustHaveTag(EC2SpotSettings{}, "BidPrice")
)

var (
	// bson fields for the MountPoint struct
	VirtualNameKey = bsonutil.MustHaveTag(MountPoint{}, "VirtualName")
	DeviceNameKey  = bsonutil.MustHaveTag(MountPoint{}, "DeviceName")
	SizeKey        = bsonutil.MustHaveTag(MountPoint{}, "Size")
)

// type/consts for price evaluation based on OS
type osType string

const (
	osLinux   osType = gcec2.DESC_LINUX_UNIX
	osSUSE    osType = "SUSE Linux"
	osWindows osType = "Windows"
)

//Utility func to create a create a temporary instance name for a host
func generateName(distroId string) string {
	return "evg_" + distroId + "_" + time.Now().Format(NameTimeFormat) +
		fmt.Sprintf("_%v", rand.New(rand.NewSource(time.Now().UnixNano())).Int())
}

// regionFullname takes the API ID of amazon region and returns the
// full region name. For instance, "us-west-1" becomes "US West (N. California)".
// This is necessary as the On Demand pricing endpoint uses the full name, unlike
// the rest of the API. THIS FUNCTION ONLY HANDLES U.S. REGIONS.
func regionFullname(region string) (string, error) {
	switch region {
	case "us-east-1":
		return "US East (N. Virginia)", nil
	case "us-west-1":
		return "US West (N. California)", nil
	case "us-west-2":
		return "US West (Oregon)", nil
	}
	return "", fmt.Errorf("region %v not supported for On Demand cost calculation", region)
}

// azToRegion takes an availability zone and returns the region id.
func azToRegion(az string) string {
	// an amazon region is just the availability zone minus the final letter
	return az[:len(az)-1]
}

// returns the format of os name expected by EC2 On Demand billing data,
// bucking the normal AWS API naming scheme.
func osBillingName(os osType) string {
	if os == osLinux {
		return "Linux"
	}
	return string(os)
}

//makeBlockDeviceMapping takes the mount_points settings defined in the distro,
//and converts them to ec2.BlockDeviceMapping structs which are usable by goamz.
//It returns a non-nil error if any of the fields appear invalid.
func makeBlockDeviceMappings(mounts []MountPoint) ([]ec2.BlockDeviceMapping, error) {
	mappings := []ec2.BlockDeviceMapping{}
	for _, mount := range mounts {
		if mount.DeviceName == "" {
			return nil, fmt.Errorf("missing 'device_name': %#v", mount)
		}
		if mount.VirtualName == "" {
			if mount.Size <= 0 {
				return nil, fmt.Errorf("invalid 'size': %#v", mount)
			}
			// EBS Storage - device name but no virtual name
			mappings = append(mappings, ec2.BlockDeviceMapping{
				DeviceName:          mount.DeviceName,
				VolumeSize:          int64(mount.Size),
				DeleteOnTermination: true,
			})
		} else {
			//Instance Storage - virtual name but no size
			mappings = append(mappings, ec2.BlockDeviceMapping{
				DeviceName:  mount.DeviceName,
				VirtualName: mount.VirtualName,
			})
		}
	}
	return mappings, nil
}

//helper function for getting an EC2 handle at US east
func getUSEast(creds aws.Auth) *ec2.EC2 {
	client := &http.Client{
		// This is the same configuration as the default in
		// net/http with the disable keep alives option specified.
		Transport: &http.Transport{
			Proxy:             http.ProxyFromEnvironment,
			DisableKeepAlives: true,
			Dial: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).Dial,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}

	return ec2.NewWithClient(creds, aws.USEast, client)
}

func getEC2KeyOptions(h *host.Host, keyPath string) ([]string, error) {
	if keyPath == "" {
		return []string{}, fmt.Errorf("No key specified for EC2 host")
	}
	opts := []string{"-i", keyPath}
	for _, opt := range h.Distro.SSHOptions {
		opts = append(opts, "-o", opt)
	}
	return opts, nil
}

//getInstanceInfo returns the full ec2 instance info for the given instance ID.
//Note that this is the *instance* id, not the spot request ID, which is different.
func getInstanceInfo(ec2Handle *ec2.EC2, instanceId string) (*ec2.Instance, error) {
	resp, err := ec2Handle.DescribeInstances([]string{instanceId}, nil)
	if err != nil {
		return nil, err
	}

	reservation := resp.Reservations
	if len(reservation) < 1 {
		err = fmt.Errorf("No reservation found for instance id: %s", instanceId)
		grip.Error(err)
		return nil, err
	}

	instances := reservation[0].Instances
	if len(instances) < 1 {
		err = fmt.Errorf("'%v' was not found in reservation '%v'",
			instanceId, resp.Reservations[0].ReservationId)
		grip.Error(err)
		return nil, err
	}

	return &instances[0], nil
}

//ec2StatusToEvergreenStatus returns a "universal" status code based on EC2's
//provider-specific status codes.
func ec2StatusToEvergreenStatus(ec2Status string) cloud.CloudStatus {
	switch ec2Status {
	case EC2StatusPending:
		return cloud.StatusInitializing
	case EC2StatusRunning:
		return cloud.StatusRunning
	case EC2StatusShuttingdown:
		return cloud.StatusTerminated
	case EC2StatusTerminated:
		return cloud.StatusTerminated
	case EC2StatusStopped:
		return cloud.StatusStopped
	default:
		return cloud.StatusUnknown
	}
}

// expireInDays creates an expire-on string in the format YYYY-MM-DD for numDays days
// in the future.
func expireInDays(numDays int) string {
	return time.Now().AddDate(0, 0, numDays).Format("2006-01-02")
}

//makeTags populates a map of tags based on a host object, which contain keys
//for the user, owner, hostname, and if it's a spawnhost or not.
func makeTags(intentHost *host.Host) map[string]string {
	// get requester host name
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// get requester user name
	var username string
	user, err := user.Current()
	if err != nil {
		username = "unknown"
	} else {
		username = user.Name
	}

	// The expire-on tag is required by MongoDB's AWS reaping policy.
	// The reaper is an external script that scans every ec2 instance for an expire-on tag,
	// and if that tag is passed the reaper terminates the host. This reaping occurs to
	// ensure that any hosts that we forget about or that fail to terminate do not stay alive
	// forever.
	expireOn := expireInDays(MciHostExpireDays)
	if intentHost.UserHost {
		// If this is a spawn host, use a different expiration date.
		expireOn = expireInDays(SpawnHostExpireDays)
	}

	tags := map[string]string{
		"Name":       intentHost.Id,
		"distro":     intentHost.Distro.Id,
		"hostname":   hostname,
		"username":   username,
		"owner":      intentHost.StartedBy,
		"mode":       "production",
		"start-time": intentHost.CreationTime.Format(NameTimeFormat),
		"expire-on":  expireOn,
	}

	if intentHost.UserHost {
		tags["mode"] = "testing"
	}
	return tags
}

//attachTags makes a call to EC2 to attach the given map of tags to a resource.
func attachTags(ec2Handle *ec2.EC2,
	tags map[string]string, instance string) error {

	tagSlice := []ec2.Tag{}
	for tag, value := range tags {
		tagSlice = append(tagSlice, ec2.Tag{tag, value})
	}

	_, err := ec2Handle.CreateTags([]string{instance}, tagSlice)
	return err
}

// determine how long until a payment is due for the specified host. since ec2
// bills per full hour the host has been up this number is just how long until,
// the host has been up the next round number of hours
func timeTilNextEC2Payment(host *host.Host) time.Duration {

	now := time.Now()

	// the time since the host was created
	timeSinceCreation := now.Sub(host.CreationTime)

	// the hours since the host was created, rounded up
	hoursRoundedUp := time.Duration(math.Ceil(timeSinceCreation.Hours()))

	// the next round number of hours the host will have been up - the time
	// that the next payment will be due
	nextPaymentTime := host.CreationTime.Add(hoursRoundedUp * time.Hour)

	return nextPaymentTime.Sub(now)

}

// ebsRegex extracts EBS Price JSON data from Amazon's UI.
var ebsRegex = regexp.MustCompile(`(?s)callback\((.*)\)`)

// ebsPriceFetcher is an interface for types capable of returning EBS price data.
// Data is in the form of map[AVAILABILITY_ZONE]PRICE.
type ebsPriceFetcher interface {
	FetchEBSPrices() (map[string]float64, error)
}

// cachedEBSPriceFetcher is a threadsafe price fetcher that only grabs EBS price
// data once during a program's execution. Prices change so infrequently that
// this is safe to do.
type cachedEBSPriceFetcher struct {
	prices map[string]float64
	m      sync.Mutex
}

// package-level price fetcher for all requests
var pkgEBSFetcher cachedEBSPriceFetcher

// FetchEBSPrices returns an EBS zone->price map. If the prices aren't cached,
// it makes a request to Amazon and caches them before returning.
func (cpf *cachedEBSPriceFetcher) FetchEBSPrices() (map[string]float64, error) {
	cpf.m.Lock()
	defer cpf.m.Unlock()
	if prices := cpf.prices; prices != nil {
		return prices, nil
	} else {
		ps, err := fetchEBSPricing()
		if err != nil {
			return nil, fmt.Errorf("fetching EBS prices: %v", err)
		}
		cpf.prices = ps
		return ps, nil
	}
}

// fetchEBSPricing does the dirty work of scraping price information from Amazon.
func fetchEBSPricing() (map[string]float64, error) {
	// there is no true EBS pricing API, so we have to wrangle it from EC2's frontend
	endpoint := "http://a0.awsstatic.com/pricing/1/ebs/pricing-ebs.js"
	grip.Debugln("Loading EBS pricing from", endpoint)
	resp, err := http.Get(endpoint)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("fetching %v: %v", endpoint, err)
	}
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %v", err)
	}
	matches := ebsRegex.FindSubmatch(data)
	if len(matches) < 2 {
		return nil, fmt.Errorf("could not find price JSON in response from %v", endpoint)
	}
	// define a one-off type for storing results from the price JSON
	prices := struct {
		Config struct {
			Regions []struct {
				Region string
				Types  []struct {
					Name   string
					Values []struct {
						Prices struct {
							USD string
						}
					}
				}
			}
		}
	}{}
	err = json.Unmarshal(matches[1], &prices)
	if err != nil {
		return nil, fmt.Errorf("parsing price JSON: %v", err)
	}

	pricePerRegion := map[string]float64{}
	for _, r := range prices.Config.Regions {
		for _, t := range r.Types {
			// only cache "general purpose" pricing for now
			if strings.Contains(t.Name, "gp2") {
				if len(t.Values) == 0 {
					continue
				}
				price, err := strconv.ParseFloat(t.Values[0].Prices.USD, 64)
				if err != nil {
					continue
				}
				pricePerRegion[r.Region] = price
			}
		}
	}
	// one final sanity check that we actually pulled information, which will alert
	// us if, say, Amazon changes the structure of their JSON
	if len(pricePerRegion) == 0 {
		return nil, fmt.Errorf("unable to parse prices from %v", endpoint)
	}
	return pricePerRegion, nil
}

// blockDeviceCosts returns the total price of a slice of BlockDevices over the given duration
// by using the EC2 API.
func blockDeviceCosts(handle *ec2.EC2, devices []ec2.BlockDevice, dur time.Duration) (float64, error) {
	cost := 0.0
	if len(devices) > 0 {
		volumeIds := []string{}
		for _, bd := range devices {
			volumeIds = append(volumeIds, bd.EBS.VolumeId)
		}
		vols, err := handle.Volumes(volumeIds, nil)
		if err != nil {
			return 0, err
		}
		for _, v := range vols.Volumes {
			// an amazon region is just the availability zone minus the final letter
			region := azToRegion(v.AvailZone)
			size, err := strconv.Atoi(v.Size)
			if err != nil {
				return 0, fmt.Errorf("reading volume size: %v", err)
			}
			p, err := ebsCost(&pkgEBSFetcher, region, size, dur)
			if err != nil {
				return 0, fmt.Errorf("EBS volume %v: %v", v.VolumeId, err)
			}
			cost += p
		}
	}
	return cost, nil
}

// ebsCost returns the cost of running an EBS block device for an amount of time in a given size and region.
// EBS bills are charged in "GB/Month" units. We consider a month to be 30 days.
func ebsCost(pf ebsPriceFetcher, region string, size int, duration time.Duration) (float64, error) {
	prices, err := pf.FetchEBSPrices()
	if err != nil {
		return 0.0, err
	}
	price, ok := prices[region]
	if !ok {
		return 0.0, fmt.Errorf("no EBS price for region '%v'", region)
	}
	// price = GB * % of month *
	month := (time.Hour * 24 * 30)
	return float64(size) * (float64(duration) / float64(month)) * price, nil
}

// onDemandPriceFetcher is an interface for fetching the hourly price of a given
// os/instance/region combination.
type onDemandPriceFetcher interface {
	FetchPrice(os osType, instance, region string) (float64, error)
}

// odInfo is an internal type for keying hosts by the attributes that affect billing.
type odInfo struct {
	os       string
	instance string
	region   string
}

// cachedOnDemandPriceFetcher is a thread-safe onDemandPriceFetcher that caches the results from
// Amazon, allowing on long load on first access followed by virtually instant response time.
type cachedOnDemandPriceFetcher struct {
	prices map[odInfo]float64
	m      sync.Mutex
}

// pkgOnDemandPriceFetcher is a package-level cached price fetcher.
// Pricing logic uses this by default to speed up price calculations.
var pkgOnDemandPriceFetcher cachedOnDemandPriceFetcher

// Terms is an internal type for loading price API results into.
type Terms struct {
	OnDemand map[string]map[string]struct {
		PriceDimensions map[string]struct {
			PricePerUnit struct {
				USD string
			}
		}
	}
}

// skuPrice digs through the incredibly verbose Amazon price data format
// for the USD dollar amount of an SKU. The for loops are for traversing
// maps of size one with an unknown key, which is simple to do in a
// language like python but really ugly here.
func (t Terms) skuPrice(sku string) float64 {
	for _, v := range t.OnDemand[sku] {
		for _, p := range v.PriceDimensions {
			// parse -- ignoring errors
			val, _ := strconv.ParseFloat(p.PricePerUnit.USD, 64)
			return val
		}
	}
	return 0
}

// FetchPrice returns the hourly price of a host based on its attributes. A pricing table
// is cached after the first communication with Amazon to avoid expensive API calls.
func (cpf *cachedOnDemandPriceFetcher) FetchPrice(os osType, instance, region string) (float64, error) {
	cpf.m.Lock()
	defer cpf.m.Unlock()
	if cpf.prices == nil {
		if err := cpf.cachePrices(); err != nil {
			return 0, fmt.Errorf("loading On Demand price data: %v", err)
		}
	}
	region, err := regionFullname(region)
	if err != nil {
		return 0, err
	}
	return cpf.prices[odInfo{
		os: osBillingName(os), instance: instance, region: region,
	}], nil
}

// cachePrices updates the internal cache with Amazon data.
func (cpf *cachedOnDemandPriceFetcher) cachePrices() error {
	cpf.prices = map[odInfo]float64{}
	// the On Demand pricing API is not part of the normal EC2 API
	endpoint := "https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/AmazonEC2/current/index.json"
	grip.Debugln("Loading On Demand pricing from", endpoint)
	resp, err := http.Get(endpoint)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("fetching %v: %v", endpoint, err)
	}
	grip.Debug("Parsing On Demand pricing")
	details := struct {
		Terms    Terms
		Products map[string]struct {
			SKU           string
			ProductFamily string
			Attributes    struct {
				Location        string
				InstanceType    string
				PreInstalledSW  string
				OperatingSystem string
				Tenancy         string
				LicenseModel    string
			}
		}
	}{}
	if err = json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return fmt.Errorf("parsing response body: %v", err)
	}

	for _, p := range details.Products {
		if p.ProductFamily == "Compute Instance" &&
			p.Attributes.PreInstalledSW == "NA" &&
			p.Attributes.Tenancy == "Shared" &&
			p.Attributes.LicenseModel != "Bring your own license" {
			// the product description does not include pricing information,
			// so we must look up the SKU in the "Terms" section.
			price := details.Terms.skuPrice(p.SKU)
			cpf.prices[odInfo{
				os:       p.Attributes.OperatingSystem,
				instance: p.Attributes.InstanceType,
				region:   p.Attributes.Location,
			}] = price
		}
	}
	return nil
}

// onDemandCost is a helper for calculating the price of an On Demand instance using the given price fetcher.
func onDemandCost(pf onDemandPriceFetcher, os osType, instance, region string, dur time.Duration) (float64, error) {
	price, err := pf.FetchPrice(os, instance, region)
	if err != nil {
		return 0, err
	}
	if price == 0 {
		return 0, fmt.Errorf("price not found in EC2 price listings")
	}
	return price * float64(dur) / float64(time.Hour), nil
}
