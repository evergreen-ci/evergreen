package ec2

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/user"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/10gen-labs/slogger/v1"
	gcec2 "github.com/dynport/gocloud/aws/ec2"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db/bsonutil"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/goamz/goamz/aws"
	"github.com/goamz/goamz/ec2"
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
	return ec2.New(creds, aws.USEast)
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
		return nil, evergreen.Logger.Errorf(slogger.ERROR, "No reservation found for "+
			"instance id: %v", instanceId)
	}

	instances := reservation[0].Instances
	if len(instances) < 1 {
		return nil, evergreen.Logger.Errorf(slogger.ERROR, "'%v' was not found in "+
			"reservation '%v'", instanceId, resp.Reservations[0].ReservationId)
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
	evergreen.Logger.Logf(slogger.DEBUG, "Loading EBS pricing from %v", endpoint)
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
