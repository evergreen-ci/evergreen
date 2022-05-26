package cloud

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/pricing"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const spotPriceCacheTTL = 2 * time.Minute

type cachingPriceFetcher struct {
	ec2Prices  map[odInfo]float64
	spotPrices map[string]cachedSpotRate
	sync.RWMutex
}

type cachedSpotRate struct {
	collectedAt time.Time
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
//	"version": "20181031070014",
//	"terms": {
//		"OnDemand": {
//			"XCH2WN4F4MYH63N8.JRTCKXETXF": {
//				"termAttributes": {
//				},
//				"sku": "XCH2WN4F4MYH63N8",
//				"priceDimensions": {
//					"XCH2WN4F4MYH63N8.JRTCKXETXF.6YS6EN2CT7": {
//						"unit": "Hrs",
//						"rateCode": "XCH2WN4F4MYH63N8.JRTCKXETXF.6YS6EN2CT7",
//						"pricePerUnit": {
//							"USD": "0.8400000000"
//						},
//						"endRange": "Inf",
//						"description": "$0.840 per Unused Reservation Linux c3.4xlarge Instance Hour",
//						"beginRange": "0",
//						"appliesTo": []
//					}
//				},
//				"offerTermCode": "JRTCKXETXF",
//				"effectiveDate": "2018-10-01T00:00:00Z"
//			}
//		}
//	},
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

// getLatestSpotCostForInstance gets the latest price for a spot instance in the given zone
// pass an empty zone to find the lowest priced zone
func (cpf *cachingPriceFetcher) getLatestSpotCostForInstance(ctx context.Context, client AWSClient, settings *EC2ProviderSettings, os osType, zone string) (float64, string, error) {
	osName := string(os)
	if settings.IsVpc {
		osName += " (Amazon VPC)"
	}

	grip.Debug(message.Fields{
		"message":       "getting spot history",
		"instance_type": settings.InstanceType,
		"function":      "getLatestSpotCostForInstance",
		"start_time":    "future",
	})

	args := hourlySpotPriceHistoryInput{
		iType: settings.InstanceType,
		os:    osType(osName),
		// passing a future start time gets the latest price only
		start: time.Now().UTC().Add(24 * time.Hour),
		end:   time.Now().UTC().Add(25 * time.Hour),
		zone:  zone,
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
	if h.UserHost || m.provider == onDemandProvider {
		h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
		return onDemandProvider, nil
	}
	if m.provider == spotProvider {
		h.Distro.Provider = evergreen.ProviderNameEc2Spot
		return spotProvider, nil
	}
	if m.provider == autoProvider {
		// The price fetcher tool is only used for the default region.
		// Suppress errors, because this isn't crucial to determining the provider type.
		if ec2settings.getRegion() == evergreen.DefaultEC2Region {
			if h.UserHost || m.provider == onDemandProvider || m.provider == autoProvider {
				onDemandPrice, err = pkgCachingPriceFetcher.getEC2OnDemandCost(ctx, m.client, getOsName(h), ec2settings.InstanceType, ec2settings.getRegion())
				grip.Error(message.WrapError(err, message.Fields{
					"message": "problem getting ec2 on-demand cost",
					"host":    h.Id,
				}))
			}
			if m.provider == spotProvider || m.provider == autoProvider {
				// passing empty zone to find the "best"
				spotPrice, az, err = pkgCachingPriceFetcher.getLatestSpotCostForInstance(ctx, m.client, ec2settings, getOsName(h), "")
				grip.Error(message.WrapError(err, message.Fields{
					"message": "problem getting latest ec2 spot price",
					"host":    h.Id,
				}))
			}
		}

		if spotPrice < onDemandPrice {
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

func getOsName(h *host.Host) osType {
	if strings.Contains(h.Distro.Arch, "windows") {
		return osWindows
	}
	return osLinux
}
