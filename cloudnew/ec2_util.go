package cloudnew

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"os/user"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/service/ec2"
	gcec2 "github.com/dynport/gocloud/aws/ec2"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// type/consts for price evaluation based on OS
type osType string

const (
	hostExpireDays             = 10
	nameTimeFormat             = "20060102150405"
	spawnHostExpireDays        = 30
	osLinux             osType = gcec2.DESC_LINUX_UNIX
	osSUSE              osType = "SUSE Linux"
	osWindows           osType = "Windows"
)

func makeBlockDeviceMappings(mounts []cloud.MountPoint) ([]*ec2.BlockDeviceMapping, error) {
	mappings := []*ec2.BlockDeviceMapping{}
	for i, mount := range mounts {
		if mount.DeviceName == "" {
			return nil, errors.Errorf("missing 'device_name': %+v", mount)
		}
		if mount.VirtualName == "" {
			return nil, errors.Errorf("missing 'virtual_name': %+v", mount)
		}
		mappings = append(mappings, &ec2.BlockDeviceMapping{
			DeviceName:  &mounts[i].DeviceName,
			VirtualName: &mounts[i].VirtualName,
		})
	}
	return mappings, nil
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
	expireOn := expireInDays(hostExpireDays)
	if intentHost.UserHost {
		// If this is a spawn host, use a different expiration date.
		expireOn = expireInDays(spawnHostExpireDays)
	}

	tags := map[string]string{
		"name":              intentHost.Id,
		"distro":            intentHost.Distro.Id,
		"evergreen-service": hostname,
		"username":          username,
		"owner":             intentHost.StartedBy,
		"mode":              "production",
		"start-time":        intentHost.CreationTime.Format(nameTimeFormat),
		"expire-on":         expireOn,
	}

	if intentHost.UserHost {
		tags["mode"] = "testing"
	}
	return tags
}

// expireInDays creates an expire-on string in the format YYYY-MM-DD for numDays days
// in the future.
func expireInDays(numDays int) string {
	return time.Now().AddDate(0, 0, numDays).Format("2006-01-02")
}

//ec2StatusToEvergreenStatus returns a "universal" status code based on EC2's
//provider-specific status codes.
func ec2StatusToEvergreenStatus(ec2Status string) cloud.CloudStatus {
	switch ec2Status {
	case cloud.EC2StatusPending:
		return cloud.StatusInitializing
	case cloud.EC2StatusRunning:
		return cloud.StatusRunning
	case cloud.EC2StatusShuttingdown:
		return cloud.StatusTerminated
	case cloud.EC2StatusTerminated:
		return cloud.StatusTerminated
	case cloud.EC2StatusStopped:
		return cloud.StatusStopped
	default:
		return cloud.StatusUnknown
	}
}

func timeTilNextEC2Payment(h *host.Host) time.Duration {
	if usesHourlyBilling(h) {
		return timeTilNextHourlyPayment(h)
	}
	return time.Second
}

func usesHourlyBilling(h *host.Host) bool { return !strings.Contains(h.Distro.Arch, "linux") }

// Determines how long until a payment is due for the specified host, for hosts
// that bill hourly. Returns the next time that it would take for the host to be
// up for an integer number of hours
func timeTilNextHourlyPayment(host *host.Host) time.Duration {
	now := time.Now()
	var startTime time.Time
	if host.StartTime.After(host.CreationTime) {
		startTime = host.StartTime
	} else {
		startTime = host.CreationTime
	}

	// the time since the host was started
	timeSinceCreation := now.Sub(startTime)

	// the hours since the host was created, rounded up
	hoursRoundedUp := time.Duration(math.Ceil(timeSinceCreation.Hours()))

	// the next round number of hours the host will have been up - the time
	// that the next payment will be due
	nextPaymentTime := startTime.Add(hoursRoundedUp * time.Hour)

	return nextPaymentTime.Sub(now)
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
	return "", errors.Errorf("region %v not supported for On Demand cost calculation", region)
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
			return nil, errors.Wrap(err, "fetching EBS prices")
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

	client := util.GetHttpClient()
	defer util.PutHttpClient(client)

	resp, err := client.Get(endpoint)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, errors.Wrapf(err, "fetching %s", endpoint)
	}
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading response body")
	}
	matches := ebsRegex.FindSubmatch(data)
	if len(matches) < 2 {
		return nil, errors.Errorf("could not find price JSON in response from %v", endpoint)
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
		return nil, errors.Wrap(err, "parsing price JSON")
	}
	fmt.Printf("%+v\n", prices)

	pricePerRegion := map[string]float64{}
	for _, r := range prices.Config.Regions {
		for _, t := range r.Types {
			// only cache "general purpose" pricing for now
			if strings.Contains(t.Name, "ebsGPSSD") {
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
		return nil, errors.Errorf("unable to parse prices from %v", endpoint)
	}
	return pricePerRegion, nil
}

// blockDeviceCosts returns the total price of a slice of BlockDevices over the given duration
// by using the EC2 API.
// ebsCost returns the cost of running an EBS block device for an amount of time in a given size and region.
// EBS bills are charged in "GB/Month" units. We consider a month to be 30 days.
func ebsCost(pf ebsPriceFetcher, region string, size int64, duration time.Duration) (float64, error) {
	prices, err := pf.FetchEBSPrices()
	if err != nil {
		return 0.0, err
	}
	price, ok := prices[region]
	if !ok {
		return 0.0, errors.Errorf("no EBS price for region '%v'", region)
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
			return 0, errors.Wrap(err, "loading On Demand price data")
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

	client := util.GetHttpClient()
	defer util.PutHttpClient(client)

	resp, err := client.Get(endpoint)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return errors.Wrapf(err, "fetching %v", endpoint)
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
		return errors.Wrap(err, "parsing response body")
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
		return 0, errors.New("price not found in EC2 price listings")
	}
	return price * dur.Hours(), nil
}

type spotRate struct {
	Time  time.Time
	Price float64
}

// spotCostForRange determines the price of a range of spot price history.
// The hostRates parameter is expected to be a slice of (time, price) pairs
// representing every hour billing cycle. The function iterates through billing
// cycles, adding up the total cost of the time span across them.
//
// This problem, incidentally, may be a good algorithms interview question ;)
func spotCostForRange(start, end time.Time, rates []spotRate) float64 {
	cost := 0.0
	cur := start
	// this loop adds up the cost of a task over all the billing periods
	// it ran within.
	for i := range rates {
		// if our start time is after the current billing range, keep skipping
		// ahead until we find the starting range.
		if i+1 < len(rates) && cur.After(rates[i+1].Time) {
			continue
		}
		// if the task's end happens before the end of this billing period,
		// we only want to calculate the cost between the billing start
		// and task end, then exit; we also do this if we're in the last rate bucket.
		if i+1 == len(rates) || end.Before(rates[i+1].Time) {
			cost += end.Sub(cur).Hours() * rates[i].Price
			break
		}
		// in the default case, we get the duration between our current time
		// and the next billing period, and multiply that duration by the current price.
		cost += rates[i+1].Time.Sub(cur).Hours() * rates[i].Price
		cur = rates[i+1].Time
	}
	return cost
}
