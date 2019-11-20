package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/pricing"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type timeRange struct {
	start time.Time
	end   time.Time
}

const spotPriceCacheTTL = 2 * time.Minute

type cachingPriceFetcher struct {
	ec2Prices  map[odInfo]float64
	ebsPrices  map[string]float64
	spotPrices map[string]cachedSpotRate
	sync.RWMutex
}

type cachedSpotRate struct {
	values      []spotRate
	collectedAt time.Time
}

func (c *cachedSpotRate) getCopy() []spotRate {
	out := make([]spotRate, len(c.values))
	copy(out, c.values)
	return out
}

// spotRate is an internal type for simplifying Amazon's price history responses.
type spotRate struct {
	Time  time.Time
	Price float64
	Zone  string
}

var pkgCachingPriceFetcher *cachingPriceFetcher

func init() {
	pkgCachingPriceFetcher = new(cachingPriceFetcher)
	pkgCachingPriceFetcher.spotPrices = make(map[string]cachedSpotRate)
}

func (cpf *cachingPriceFetcher) getEC2Cost(ctx context.Context, client AWSClient, h *host.Host, t timeRange) (float64, error) {
	dur := t.end.Sub(t.start)
	if h.ComputeCostPerHour > 0 {
		grip.Debug(message.Fields{
			"message":               "returning cost data cached in host",
			"host_id":               h.Id,
			"compute_cost_per_hour": h.ComputeCostPerHour,
		})
		return h.ComputeCostPerHour * dur.Hours(), nil
	}
	os := getOsName(h)
	if isHostOnDemand(h) {
		zone, err := getZone(ctx, client, h)
		if err != nil {
			return 0, errors.Wrap(err, "could not get zone for host")
		}
		region := AztoRegion(zone)
		price, err := cpf.getEC2OnDemandCost(ctx, client, os, h.InstanceType, region)
		if err != nil {
			return 0, err
		}
		return price * dur.Hours(), nil
	}
	return cpf.calculateSpotCost(ctx, client, h, os, t)
}

// TODO: Remove this function in favor of just using h.Zone once all running EC2 hosts have h.Zone set.
func getZone(ctx context.Context, client AWSClient, h *host.Host) (string, error) {
	if h.Zone != "" {
		return h.Zone, nil
	}
	instanceID := h.Id
	if h.ExternalIdentifier != "" {
		instanceID = h.ExternalIdentifier
	}
	instance, err := client.GetInstanceInfo(ctx, instanceID)
	if err != nil {
		return "", errors.Wrap(err, "error getting instance info")
	}
	return *instance.Placement.AvailabilityZone, nil
}

func (cpf *cachingPriceFetcher) getEC2OnDemandCost(ctx context.Context, client AWSClient, osEC2Name osType, instance, region string) (float64, error) {
	if cpf.ec2Prices == nil {
		cpf.Lock()
		cpf.ec2Prices = map[odInfo]float64{}
		cpf.Unlock()
	}

	// convert to pricing api strings
	osPriceName := osBillingName(osEC2Name)
	region, err := regionFullname(region)
	if err != nil {
		return 0, err
	}

	cpf.RLock()
	if val, ok := cpf.ec2Prices[odInfo{
		os: osPriceName, instance: instance, region: region,
	}]; ok {
		cpf.RUnlock()
		return val, nil
	}
	cpf.RUnlock()

	getProductsInput := cpf.makeGetProductsInput(odInfo{os: osPriceName, instance: instance, region: region})
	out, err := client.GetProducts(ctx, getProductsInput)
	if err != nil {
		return 0, errors.Wrap(err, "problem querying for pricing data")
	}

	p, err := cpf.parseAWSPricing(out)
	if err != nil {
		return 0, errors.Wrap(err, "problem parsing aws pricing")
	}
	cpf.Lock()
	defer cpf.Unlock()
	cpf.ec2Prices[odInfo{os: osPriceName, instance: instance, region: region}] = p
	return p, nil
}

func (cpf *cachingPriceFetcher) makeGetProductsInput(info odInfo) *pricing.GetProductsInput {
	const match = "TERM_MATCH"
	constructGetProductsInput := &pricing.GetProductsInput{
		Filters: []*pricing.Filter{
			{
				Field: aws.String("ServiceCode"),
				Type:  aws.String(match),
				Value: aws.String("AmazonEC2"),
			},
			{
				Field: aws.String("productFamily"),
				Type:  aws.String(match),
				Value: aws.String("Compute Instance"),
			},
			{
				Field: aws.String("preInstalledSw"),
				Type:  aws.String(match),
				Value: aws.String("NA"),
			},
			{
				Field: aws.String("tenancy"),
				Type:  aws.String(match),
				Value: aws.String("Shared"),
			},
			{
				Field: aws.String("capacityStatus"),
				Type:  aws.String(match),
				Value: aws.String("UnusedCapacityReservation"),
			},
			{
				Field: aws.String("instanceType"),
				Type:  aws.String(match),
				Value: aws.String(info.instance),
			},
			{
				Field: aws.String("operatingSystem"),
				Type:  aws.String(match),
				Value: aws.String(info.os),
			},
			{
				Field: aws.String("location"),
				Type:  aws.String(match),
				Value: aws.String(info.region),
			},
		},
		ServiceCode: aws.String("AmazonEC2"),
	}
	return constructGetProductsInput
}

// Parse a JSON document like this one
// {
// 	"version": "20181031070014",
// 	"terms": {
// 		"OnDemand": {
// 			"XCH2WN4F4MYH63N8.JRTCKXETXF": {
// 				"termAttributes": {
// 				},
// 				"sku": "XCH2WN4F4MYH63N8",
// 				"priceDimensions": {
// 					"XCH2WN4F4MYH63N8.JRTCKXETXF.6YS6EN2CT7": {
// 						"unit": "Hrs",
// 						"rateCode": "XCH2WN4F4MYH63N8.JRTCKXETXF.6YS6EN2CT7",
// 						"pricePerUnit": {
// 							"USD": "0.8400000000"
// 						},
// 						"endRange": "Inf",
// 						"description": "$0.840 per Unused Reservation Linux c3.4xlarge Instance Hour",
// 						"beginRange": "0",
// 						"appliesTo": []
// 					}
// 				},
// 				"offerTermCode": "JRTCKXETXF",
// 				"effectiveDate": "2018-10-01T00:00:00Z"
// 			}
// 		}
// 	},
// [...]
func (cpf *cachingPriceFetcher) parseAWSPricing(out *pricing.GetProductsOutput) (float64, error) {
	if len(out.PriceList) != 1 {
		return 0, errors.Errorf("problem parsing price list %v", out.PriceList)
	}
	terms, ok := out.PriceList[0]["terms"]
	if !ok {
		return 0, errors.Errorf("problem parsing price list %v", out.PriceList)
	}
	termsMap, ok := terms.(map[string]interface{})
	if !ok {
		return 0, errors.Errorf("problem parsing price list %v", out.PriceList)
	}
	onDemand, ok := termsMap["OnDemand"]
	if !ok {
		return 0, errors.Errorf("problem parsing price list %v", out.PriceList)
	}
	onDemandMap, ok := onDemand.(map[string]interface{})
	if !ok {
		return 0, errors.Errorf("problem parsing price list %v", out.PriceList)
	}
	var priceDimensionMap map[string]interface{}
	for _, v := range onDemandMap {
		onDemandMapComponent, ok := v.(map[string]interface{})
		if !ok {
			return 0, errors.Errorf("problem parsing price list %v", out.PriceList)
		}
		priceDimension, ok := onDemandMapComponent["priceDimensions"]
		if !ok {
			return 0, errors.Errorf("problem parsing price list %v", out.PriceList)
		}
		priceDimensionMap, ok = priceDimension.(map[string]interface{})
		if !ok {
			return 0, errors.Errorf("problem parsing price list %v", out.PriceList)
		}
	}
	var price string
	for _, v := range priceDimensionMap {
		priceDimensionComponent, ok := v.(map[string]interface{})
		if !ok {
			return 0, errors.Errorf("problem parsing price list %v", out.PriceList)
		}
		pricePerUnit, ok := priceDimensionComponent["pricePerUnit"]
		if !ok {
			return 0, errors.Errorf("problem parsing price list %v", out.PriceList)
		}
		pricePerUnitMap, ok := pricePerUnit.(map[string]interface{})
		if !ok {
			return 0, errors.Errorf("problem parsing price list %v", out.PriceList)
		}
		USD, ok := pricePerUnitMap["USD"]
		if !ok {
			return 0, errors.Errorf("problem parsing price list %v", out.PriceList)
		}
		price, ok = USD.(string)
		if !ok {
			return 0, errors.Errorf("problem parsing price list %v", out.PriceList)
		}
	}
	p, err := strconv.ParseFloat(price, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "problem parsing %s as int", price)
	}
	return p, nil
}

func (cpf *cachingPriceFetcher) getLatestLowestSpotCostForInstance(ctx context.Context, client AWSClient, settings *EC2ProviderSettings, os osType) (float64, string, error) {
	osName := string(os)
	if settings.IsVpc {
		osName += " (Amazon VPC)"
	}

	grip.Debug(message.Fields{
		"message":       "getting spot history",
		"instance_type": settings.InstanceType,
		"function":      "getLatestLowestSpotCostForInstance",
		"start_time":    "future",
	})

	args := hourlySpotPriceHistoryInput{
		iType: settings.InstanceType,
		os:    osType(osName),
		// passing a future start time gets the latest price only
		start: time.Now().UTC().Add(24 * time.Hour),
		end:   time.Now().UTC().Add(25 * time.Hour),
		// passing empty zone to find the "best"
		zone: "",
	}

	prices, err := cpf.describeSpotPriceHistory(ctx, client, args)
	if err != nil {
		return 0, "", errors.WithStack(err)
	}
	if len(prices) == 0 {
		return 0, "", errors.New("no prices found")
	}

	var min float64
	var az string
	for i := range prices {
		p, err := strconv.ParseFloat(*prices[i].SpotPrice, 64)
		if err != nil {
			return 0, "", errors.Wrapf(err, "problem parsing %s", *prices[i].SpotPrice)
		}
		if min == 0 || p < min {
			min = p
			az = *prices[i].AvailabilityZone
		}
	}
	return min, az, nil
}

func (m *ec2Manager) getProvider(ctx context.Context, h *host.Host, ec2settings *EC2ProviderSettings) (ec2ProviderType, error) {
	var (
		err           error
		onDemandPrice float64
		spotPrice     float64
		az            string
	)
	if h.UserHost || m.provider == onDemandProvider || m.provider == autoProvider {
		ec2Settings := &EC2ProviderSettings{}
		err := ec2Settings.fromDistroSettings(h.Distro)
		if err != nil {
			return 0, errors.Wrap(err, "problem getting settings from host")
		}
		onDemandPrice, err = pkgCachingPriceFetcher.getEC2OnDemandCost(ctx, m.client, getOsName(h), ec2settings.InstanceType, ec2settings.getRegion())
		if err != nil {
			return 0, errors.Wrap(err, "error getting ec2 on-demand cost")
		}
	}
	if m.provider == spotProvider || m.provider == autoProvider {
		spotPrice, az, err = pkgCachingPriceFetcher.getLatestLowestSpotCostForInstance(ctx, m.client, ec2settings, getOsName(h))
		if err != nil {
			return 0, errors.Wrap(err, "error getting latest lowest spot price")
		}
	}
	if h.UserHost || m.provider == onDemandProvider {
		h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
		h.ComputeCostPerHour = onDemandPrice
		return onDemandProvider, nil
	}
	if m.provider == spotProvider {
		h.Distro.Provider = evergreen.ProviderNameEc2Spot
		h.ComputeCostPerHour = spotPrice
		return spotProvider, nil
	}
	if m.provider == autoProvider {
		if spotPrice < onDemandPrice {
			h.ComputeCostPerHour = spotPrice
			ec2settings.BidPrice = onDemandPrice
			if ec2settings.VpcName != "" {
				subnetID, err := m.getSubnetForAZ(ctx, az, ec2settings.VpcName)
				if err != nil {
					return 0, errors.Wrap(err, "error settings dynamic subnet for spot")
				}
				ec2settings.SubnetId = subnetID
			}
			h.Distro.Provider = evergreen.ProviderNameEc2Spot
			return spotProvider, nil
		}
		h.ComputeCostPerHour = onDemandPrice
		h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
		return onDemandProvider, nil
	}
	return 0, errors.Errorf("provider is %d, expected %d, %d, or %d", m.provider, onDemandProvider, spotProvider, autoProvider)
}

func (m *ec2Manager) getSubnetForAZ(ctx context.Context, azName, vpcName string) (string, error) {
	vpcs, err := m.client.DescribeVpcs(ctx, &ec2.DescribeVpcsInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name: aws.String("tag:Name"),
				Values: []*string{
					aws.String(vpcName),
				},
			},
		},
	})
	if err != nil {
		return "", errors.Wrap(err, "error finding vpc id")
	}
	vpcID := *vpcs.Vpcs[0].VpcId

	subnets, err := m.client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(vpcID)},
			},
			&ec2.Filter{
				Name:   aws.String("availability-zone"),
				Values: []*string{aws.String(azName)},
			},
			&ec2.Filter{
				Name:   aws.String("tag:Name"),
				Values: []*string{aws.String(vpcName + ".subnet_" + strings.Split(azName, "-")[2])},
			},
		},
	})
	if err != nil {
		return "", errors.Wrap(err, "error finding subnet id")
	}
	return *subnets.Subnets[0].SubnetId, nil
}

func (cpf *cachingPriceFetcher) getEBSCost(ctx context.Context, client AWSClient, h *host.Host, t timeRange) (float64, error) {
	cpf.Lock()
	defer cpf.Unlock()
	var err error
	dur := t.end.Sub(t.start)
	size, err := getVolumeSize(ctx, client, h)
	if err != nil {
		return 0, errors.Wrap(err, "error getting volume size")
	}
	zone, err := getZone(ctx, client, h)
	if err != nil {
		return 0, errors.Wrap(err, "could not get zone for host")
	}
	region := AztoRegion(zone)
	return cpf.ebsCost(region, size, dur)
}

func getVolumeSize(ctx context.Context, client AWSClient, h *host.Host) (int64, error) {
	if h.VolumeTotalSize != 0 {
		return h.VolumeTotalSize, nil
	}

	volumeIDs, err := client.GetVolumeIDs(ctx, h)
	if err != nil {
		return 0, errors.Wrapf(err, "can't get volume IDs for '%s'", h.Id)
	}
	vols, err := client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
		VolumeIds: aws.StringSlice(volumeIDs),
	})
	if err != nil {
		return 0, errors.Wrap(err, "error describing volumes")
	}

	var totalSize int64
	for _, v := range vols.Volumes {
		totalSize += *v.Size
	}

	return totalSize, nil
}

// ebsCost returns the cost of running an EBS block device for an amount of time in a given size and region.
// EBS bills are charged in "GB/Month" units. We consider a month to be 30 days.
func (cpf *cachingPriceFetcher) ebsCost(region string, size int64, duration time.Duration) (float64, error) {
	if cpf.ebsPrices == nil {
		if err := cpf.cacheEBSPrices(); err != nil {
			return 0, errors.Wrap(err, "error fetching EBS prices")
		}
	}
	price, ok := cpf.ebsPrices[region]
	if !ok {
		return 0.0, errors.Errorf("no EBS price for region '%v'", region)
	}
	// price = GB * % of month *
	month := (time.Hour * 24 * 30)
	return float64(size) * (float64(duration) / float64(month)) * price, nil

}

// fetchEBSPricing does the dirty work of scraping price information from Amazon.
func (cpf *cachingPriceFetcher) cacheEBSPrices() error {
	// there is no true EBS pricing API, so we have to wrangle it from EC2's frontend
	endpoint := "http://a0.awsstatic.com/pricing/1/ebs/pricing-ebs.js"
	grip.Debugln("Loading EBS pricing from", endpoint)

	client := util.GetHTTPClient()
	defer util.PutHTTPClient(client)

	var data []byte

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := util.Retry(
		ctx,
		func() (bool, error) {
			resp, err := client.Get(endpoint)
			if resp != nil {
				defer resp.Body.Close()
			}
			if err != nil {
				return true, errors.Wrapf(err, "fetching %s", endpoint)
			}
			data, err = ioutil.ReadAll(resp.Body)
			if err != nil {
				return true, errors.Wrap(err, "reading response body")
			}
			return false, nil
		}, awsClientImplRetries, awsClientImplStartPeriod, 0)
	if err != nil {
		return errors.WithStack(err)
	}

	matches := ebsRegex.FindSubmatch(data)
	if len(matches) < 2 {
		return errors.Errorf("could not find price JSON in response from %v", endpoint)
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
		return errors.Wrap(err, "parsing price JSON")
	}

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
		return errors.Errorf("unable to parse prices from %v", endpoint)
	}
	cpf.ebsPrices = pricePerRegion
	return nil
}

// calculateSpotCost is a helper for fetching spot price history and computing the
// cost of a task across a host's billing cycles.
func (cpf *cachingPriceFetcher) calculateSpotCost(ctx context.Context, client AWSClient, h *host.Host, os osType, t timeRange) (float64, error) {
	zone, err := getZone(ctx, client, h)
	if err != nil {
		return 0, errors.Wrap(err, "could not get zone for host")
	}
	rates, err := cpf.describeHourlySpotPriceHistory(ctx, client, hourlySpotPriceHistoryInput{
		iType: h.InstanceType,
		zone:  zone,
		os:    os,
		start: h.StartTime,
		end:   t.end,
	})
	if err != nil {
		return 0, errors.Wrap(err, "error getting hourly spot price history")
	}
	return spotCostForRange(t.start, t.end, rates), nil
}

type hourlySpotPriceHistoryInput struct {
	iType string
	zone  string
	os    osType
	start time.Time
	end   time.Time
}

func (i hourlySpotPriceHistoryInput) String() string {
	return fmt.Sprintln(i.iType, i.zone, i.os, i.start.Round(spotPriceCacheTTL).Unix())
}

// describeHourlySpotPriceHistory talks to Amazon to get spot price history
func (cpf *cachingPriceFetcher) describeSpotPriceHistory(ctx context.Context, client AWSClient, input hourlySpotPriceHistoryInput) ([]*ec2.SpotPrice, error) {
	cpf.Lock()
	defer cpf.Unlock()

	cleanedNum := 0
	for k, v := range cpf.spotPrices {
		if time.Since(v.collectedAt) > spotPriceCacheTTL {
			cleanedNum++
			delete(cpf.spotPrices, k)
		}
	}
	grip.DebugWhen(cleanedNum > 0, message.Fields{
		"message":   "cleaned cached spot prices",
		"ttl_secs":  spotPriceCacheTTL.Seconds(),
		"expired":   cleanedNum,
		"remaining": len(cpf.spotPrices),
	})

	// expand times to contain the full runtime of the host
	startFilter, endFilter := input.start.Add(-time.Hour), input.end.Add(time.Hour)
	osStr := string(input.os)
	grip.Debug(message.Fields{
		"instance_type": &input.iType,
		"start_time":    &startFilter,
		"end_time":      &endFilter,
		"function":      "describeHourlySpotPriceHistory",
	})
	filter := &ec2.DescribeSpotPriceHistoryInput{
		InstanceTypes:       []*string{&input.iType},
		ProductDescriptions: []*string{&osStr},
		AvailabilityZone:    &input.zone,
		StartTime:           &startFilter,
		EndTime:             &endFilter,
	}
	// iterate through all pages of results (the helper that does this for us appears to be broken)
	history := []*ec2.SpotPrice{}
	for {
		h, err := client.DescribeSpotPriceHistory(ctx, filter)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		history = append(history, h.SpotPriceHistory...)
		if h.NextToken != nil && *h.NextToken != "" {
			filter.NextToken = h.NextToken
		} else {
			break
		}
	}
	return history, nil
}

// describeHourlySpotPriceHistory simplifies that spot price history into hourly billing rates
// starting from the supplied start time. Returns a slice of hour-separated spot prices.
func (cpf *cachingPriceFetcher) describeHourlySpotPriceHistory(ctx context.Context, client AWSClient, input hourlySpotPriceHistoryInput) ([]spotRate, error) {
	cacheKey := input.String()
	cpf.RLock()
	cachedValue, ok := cpf.spotPrices[cacheKey]
	l := len(cpf.spotPrices)
	cpf.RUnlock()
	if ok {
		staleFor := time.Since(cachedValue.collectedAt)
		if staleFor < spotPriceCacheTTL && len(cachedValue.values) > 0 {
			grip.Debug(message.Fields{
				"message":     "found spot price in cache",
				"cached_secs": staleFor.Seconds(),
				"key":         cacheKey,
				"cache_size":  l,
			})

			return cachedValue.getCopy(), nil
		}
	}
	history, err := cpf.describeSpotPriceHistory(ctx, client, input)
	if err != nil {
		return nil, errors.Wrap(err, "problem getting spot price history")
	}

	// this loop samples the spot price history (which includes updates for every few minutes)
	// into hourly billing periods. The price we are billed for an hour of spot time is the
	// current price at the start of the hour. Amazon returns spot price history sorted in
	// decreasing time order. We iterate backwards through the list to
	// pretend the ordering to increasing time.
	prices := []spotRate{}
	i := len(history) - 1
	for i >= 0 {
		// add the current hourly price if we're in the last result bucket
		// OR our billing hour starts the same time as the data (very rare)
		// OR our billing hour starts after the current bucket but before the next one
		if i == 0 || input.start.Equal(*history[i].Timestamp) ||
			input.start.After(*history[i].Timestamp) && input.start.Before(*history[i-1].Timestamp) {
			price, err := strconv.ParseFloat(*history[i].SpotPrice, 64)
			if err != nil {
				return nil, errors.Wrap(err, "parsing spot price")
			}
			var zone string
			if history[i].AvailabilityZone != nil {
				zone = *history[i].AvailabilityZone
			}

			prices = append(prices, spotRate{Time: input.start, Price: price, Zone: zone})
			// we increment the hour but stay on the same price history index
			// in case the current spot price spans more than one hour
			input.start = input.start.Add(time.Hour)
			if input.start.After(input.end) {
				break
			}
		} else {
			// continue iterating through our price history whenever we
			// aren't matching the next billing hour
			i--
		}
	}

	cachedValue = cachedSpotRate{
		collectedAt: time.Now(),
		values:      prices,
	}
	cpf.Lock()
	defer cpf.Unlock()
	cpf.spotPrices[cacheKey] = cachedValue
	return cachedValue.getCopy(), nil
}

func getOsName(h *host.Host) osType {
	if strings.Contains(h.Distro.Arch, "windows") {
		return osWindows
	}
	return osLinux
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
