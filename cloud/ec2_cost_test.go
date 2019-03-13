// +build !race

package cloud

import (
	"context"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type CostUnitSuite struct {
	suite.Suite
	rates []spotRate
}

func TestCostUnitSuite(t *testing.T) {
	suite.Run(t, new(CostUnitSuite))
}

// mins returns a time X minutes after UNIX epoch
func mins(x int64) time.Time {
	return time.Unix(60*x, 0)
}

func (s *CostUnitSuite) SetupTest() {
	s.rates = []spotRate{
		{Time: mins(0), Price: 1.0},
		{Time: mins(60), Price: .5},
		{Time: mins(2 * 60), Price: 1.0},
		{Time: mins(3 * 60), Price: 2.0},
		{Time: mins(4 * 60), Price: 1.0},
	}
}

func (s *CostUnitSuite) TestSpotCostForRange() {
	price := spotCostForRange(mins(10), mins(40), s.rates)
	s.Equal(.5, price)
	price = spotCostForRange(mins(10+4*60), mins(40+4*60), s.rates)
	s.Equal(.5, price)
	price = spotCostForRange(mins(45), mins(75), s.rates)
	s.Equal(.375, price)
	price = spotCostForRange(mins(30), mins(150), s.rates)
	s.Equal(1.5, price)
	price = spotCostForRange(mins(4*60+30), mins(5*60), s.rates)
	s.Equal(.5, price)
	price = spotCostForRange(mins(4*60), mins(5*60), s.rates)
	s.Equal(1.0, price)
	price = spotCostForRange(mins(0), mins(30), s.rates)
	s.Equal(.5, price)
	price = spotCostForRange(mins(-60), mins(45), s.rates)
	s.Equal(1.75, price)
}

func (s *CostUnitSuite) TestEBSCostCalculation() {
	region := "X"
	cpf := &cachingPriceFetcher{
		ebsPrices: map[string]float64{
			region: 1.00,
		},
	}
	cost, err := cpf.ebsCost(region, 1, time.Hour*24*30)
	s.NoError(err)
	s.Equal(1.00, cost)
	cost, err = cpf.ebsCost(region, 20, time.Hour*24*30)
	s.NoError(err)
	s.Equal(20.0, cost)
	cost, err = cpf.ebsCost(region, 100, time.Hour)
	s.NoError(err)
	s.InDelta(.135, cost, .005)
	cost, err = cpf.ebsCost(region, 100, time.Minute*20)
	s.NoError(err)
	s.InDelta(.045, cost, .005)

	cpf = &cachingPriceFetcher{ebsPrices: map[string]float64{}}
	_, err = cpf.ebsCost("mars-west-1", 100, time.Minute*20)
	s.Error(err)
}

func (s *CostUnitSuite) TestOnDemandPriceAPITranslation() {
	s.Equal("Linux", osBillingName(osLinux))
	s.Equal(string(osSUSE), osBillingName(osSUSE))
	s.Equal(string(osWindows), osBillingName(osWindows))
	r, err := regionFullname("us-east-1")
	s.NoError(err)
	s.Equal("US East (N. Virginia)", r)
	r, err = regionFullname("us-west-1")
	s.NoError(err)
	s.Equal("US West (N. California)", r)
	r, err = regionFullname("us-west-2")
	s.NoError(err)
	s.Equal("US West (Oregon)", r)
	_, err = regionFullname("amazing")
	s.Error(err)
}

func (s *CostUnitSuite) TestOnDemandPriceCalculation() {
	client := &awsClientMock{}
	h := &host.Host{InstanceType: "m3.4xlarge"}
	h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
	r, _ := regionFullname("us-east-1")
	cpf := &cachingPriceFetcher{
		ec2Prices: map[odInfo]float64{
			odInfo{
				"Linux",
				"m3.4xlarge",
				r,
			}: 1.0,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	now := time.Now()
	cost, err := cpf.getEC2Cost(ctx, client, h, timeRange{now.Add(-30 * time.Minute), now})
	s.NoError(err)
	s.Equal(.50, cost)
	cost, err = cpf.getEC2Cost(ctx, client, h, timeRange{now.Add(-time.Hour), now})
	s.NoError(err)
	s.Equal(1.0, cost)
	cost, err = cpf.getEC2Cost(ctx, client, h, timeRange{now.Add(-2 * time.Hour), now})
	s.NoError(err)
	s.Equal(2.0, cost)
}

func (s *CostUnitSuite) TestTimeTilNextPayment() {
	hourlyHost := host.Host{
		Id: "hourlyHost",
		Distro: distro.Distro{
			Arch: "windows_amd64",
		},
		CreationTime: time.Date(2017, 1, 1, 0, 30, 0, 0, time.Local),
		StartTime:    time.Date(2017, 1, 1, 1, 0, 0, 0, time.Local),
	}
	secondlyHost := host.Host{
		Id: "secondlyHost",
		Distro: distro.Distro{
			Arch: "linux_amd64",
		},
		CreationTime: time.Date(2017, 1, 1, 0, 0, 0, 0, time.Local),
		StartTime:    time.Date(2017, 1, 1, 0, 30, 0, 0, time.Local),
	}
	hourlyHostNoStartTime := host.Host{
		Id: "hourlyHostNoStartTime",
		Distro: distro.Distro{
			Arch: "windows_amd64",
		},
		CreationTime: time.Date(2017, 1, 1, 0, 0, 0, 0, time.Local),
	}
	now := time.Now()
	timeTilNextHour := int(time.Hour) - (now.Minute()*int(time.Minute) + now.Second()*int(time.Second) + now.Nanosecond()*int(time.Nanosecond))

	timeNextPayment := timeTilNextEC2Payment(&hourlyHost)
	s.InDelta(timeTilNextHour, timeNextPayment.Nanoseconds(), float64(1*time.Millisecond))

	timeNextPayment = timeTilNextEC2Payment(&secondlyHost)
	s.InDelta(1*time.Second, timeNextPayment.Nanoseconds(), float64(1*time.Millisecond))

	timeNextPayment = timeTilNextEC2Payment(&hourlyHostNoStartTime)
	s.InDelta(timeTilNextHour, timeNextPayment.Nanoseconds(), float64(1*time.Millisecond))
}

func (s *CostUnitSuite) TestGetLatestSpotCostsForInstance() {
	client := &awsClientMock{}
	instanceType := "m4.large"
	now := time.Now()
	spotPrices := []struct {
		zone  string
		price string
	}{
		{zone: "us-east-1a", price: "0.5"},
		{zone: "us-east-1b", price: "0.2"},
		{zone: "us-east-1c", price: "1.3"},
		{zone: "us-east-1d", price: "0.3"},
	}

	priceHistory := make([]*ec2.SpotPrice, 0, len(spotPrices))
	for i := range spotPrices {
		priceHistory = append(priceHistory, &ec2.SpotPrice{
			AvailabilityZone: &spotPrices[i].zone,
			SpotPrice:        &spotPrices[i].price,
			InstanceType:     &instanceType,
			Timestamp:        &now,
		})
	}
	client.DescribeSpotPriceHistoryOutput = &ec2.DescribeSpotPriceHistoryOutput{SpotPriceHistory: priceHistory}

	cpf := &cachingPriceFetcher{}

	settings := &EC2ProviderSettings{InstanceType: instanceType}

	zonePrices, err := cpf.getLatestSpotCostsForInstance(context.TODO(), client, settings, "fooOS")
	s.NoError(err)
	s.Equal(len(spotPrices), len(zonePrices))
	s.True(sort.IsSorted(zonePrices))

	p0, err := strconv.ParseFloat(spotPrices[0].price, 64)
	s.NoError(err)
	s.Equal(p0, zonePrices[2].price)

	p1, err := strconv.ParseFloat(spotPrices[1].price, 64)
	s.NoError(err)
	s.Equal(p1, zonePrices[0].price)

	p2, err := strconv.ParseFloat(spotPrices[2].price, 64)
	s.NoError(err)
	s.Equal(p2, zonePrices[3].price)

	p3, err := strconv.ParseFloat(spotPrices[3].price, 64)
	s.NoError(err)
	s.Equal(p3, zonePrices[1].price)
}

type CostIntegrationSuite struct {
	suite.Suite
	m      *ec2Manager
	client AWSClient
}

func TestCostIntegrationSuite(t *testing.T) {
	suite.Run(t, new(CostIntegrationSuite))
}

func (s *CostIntegrationSuite) SetupSuite() {
	settings := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(s.T(), settings, "CostIntegrationSuite")

	m := NewEC2Manager(&EC2ManagerOptions{client: &awsClientImpl{}})
	s.m = m.(*ec2Manager)
	s.NoError(s.m.Configure(context.TODO(), settings))
	s.NoError(s.m.client.Create(s.m.credentials, defaultRegion))
	s.client = s.m.client
}

func (s *CostIntegrationSuite) SetupTest() {
	pkgCachingPriceFetcher.ec2Prices = nil
}

func (s *CostIntegrationSuite) TestSpotPriceHistory() {
	cpf := cachingPriceFetcher{
		spotPrices: make(map[string]cachedSpotRate),
	}
	input := hourlySpotPriceHistoryInput{
		iType: "m3.large",
		zone:  "us-east-1a",
		os:    osLinux,
		start: time.Now().Add(-2 * time.Hour),
		end:   time.Now(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ps, err := cpf.describeHourlySpotPriceHistory(ctx, s.client, input)
	s.NoError(err)
	s.Require().True(len(ps) > 2)
	s.True(ps[len(ps)-1].Time.Before(time.Now()))
	s.True(ps[len(ps)-1].Time.After(time.Now().Add(-10 * time.Minute)))
	s.True(ps[0].Price > 0.0)
	s.True(ps[0].Price < 2.0)
	s.True(ps[0].Time.Before(ps[1].Time))

	input = hourlySpotPriceHistoryInput{
		iType: "m3.large",
		zone:  "us-east-1a",
		os:    osLinux,
		start: time.Now().Add(-240 * time.Hour),
		end:   time.Now(),
	}
	ps, err = cpf.describeHourlySpotPriceHistory(ctx, s.client, input)
	s.NoError(err)
	s.True(len(ps) > 240, "num_elems: %d", len(ps))
	s.True(ps[len(ps)-1].Time.Before(time.Now()))
	s.True(ps[len(ps)-1].Time.After(time.Now().Add(-30 * time.Minute)))
	s.True(ps[0].Time.After(time.Now().Add(-242 * time.Hour)))
	s.True(ps[0].Time.Before(time.Now().Add(-240 * time.Hour)))
	s.True(ps[0].Price > 0.0)
	s.True(ps[0].Price < 2.0)
	s.True(ps[0].Time.Before(ps[1].Time))
}

func (s *CostIntegrationSuite) TestFetchEBSPricing() {
	cpf := cachingPriceFetcher{}
	price, err := cpf.ebsCost("us-east-1", 1, time.Hour*24*30)
	s.NoError(err)
	s.True(price > 0)
}

func (s *CostIntegrationSuite) TestEBSPriceCaching() {
	cpf := cachingPriceFetcher{}
	s.Nil(cpf.ebsPrices)
	err := cpf.cacheEBSPrices()
	s.NoError(err)
	s.NotNil(cpf.ebsPrices)
}

func (s *CostIntegrationSuite) TestFetchOnDemandPricingCached() {
	pkgCachingPriceFetcher.ec2Prices = map[odInfo]float64{
		odInfo{os: "Linux", instance: "c3.4xlarge", region: "US East (N. Virginia)"}:   .1,
		odInfo{os: "Windows", instance: "c3.4xlarge", region: "US East (N. Virginia)"}: .2,
		odInfo{os: "Linux", instance: "c3.xlarge", region: "US East (N. Virginia)"}:    .3,
		odInfo{os: "Windows", instance: "c3.xlarge", region: "US East (N. Virginia)"}:  .4,
		odInfo{os: "Linux", instance: "m5.4xlarge", region: "US East (N. Virginia)"}:   .5,
		odInfo{os: "Windows", instance: "m5.4xlarge", region: "US East (N. Virginia)"}: .6,
		odInfo{os: "Linux", instance: "m5.xlarge", region: "US East (N. Virginia)"}:    .7,
		odInfo{os: "Windows", instance: "m5.xlarge", region: "US East (N. Virginia)"}:  .8,
	}

	price, err := pkgCachingPriceFetcher.getEC2OnDemandCost(context.Background(), s.m.client, osLinux, "c3.4xlarge", "us-east-1")
	s.NoError(err)
	s.Equal(.1, price)

	price, err = pkgCachingPriceFetcher.getEC2OnDemandCost(context.Background(), s.m.client, osWindows, "c3.4xlarge", "us-east-1")
	s.NoError(err)
	s.Equal(.2, price)

	price, err = pkgCachingPriceFetcher.getEC2OnDemandCost(context.Background(), s.m.client, osLinux, "c3.xlarge", "us-east-1")
	s.NoError(err)
	s.Equal(.3, price)

	price, err = pkgCachingPriceFetcher.getEC2OnDemandCost(context.Background(), s.m.client, osWindows, "c3.xlarge", "us-east-1")
	s.NoError(err)
	s.Equal(.4, price)

	price, err = pkgCachingPriceFetcher.getEC2OnDemandCost(context.Background(), s.m.client, osLinux, "m5.4xlarge", "us-east-1")
	s.NoError(err)
	s.Equal(.5, price)

	price, err = pkgCachingPriceFetcher.getEC2OnDemandCost(context.Background(), s.m.client, osWindows, "m5.4xlarge", "us-east-1")
	s.NoError(err)
	s.Equal(.6, price)

	price, err = pkgCachingPriceFetcher.getEC2OnDemandCost(context.Background(), s.m.client, osLinux, "m5.xlarge", "us-east-1")
	s.NoError(err)
	s.Equal(.7, price)

	price, err = pkgCachingPriceFetcher.getEC2OnDemandCost(context.Background(), s.m.client, osWindows, "m5.xlarge", "us-east-1")
	s.NoError(err)
	s.Equal(.8, price)
}

func (s *CostIntegrationSuite) TestFetchOnDemandPricingUncached() {
	price, err := pkgCachingPriceFetcher.getEC2OnDemandCost(context.Background(), s.m.client, osLinux, "c3.4xlarge", "us-east-1")
	s.NoError(err)
	s.Equal(.84, price)

	price, err = pkgCachingPriceFetcher.getEC2OnDemandCost(context.Background(), s.m.client, osWindows, "c3.4xlarge", "us-east-1")
	s.NoError(err)
	s.Equal(1.504, price)

	price, err = pkgCachingPriceFetcher.getEC2OnDemandCost(context.Background(), s.m.client, osLinux, "c3.xlarge", "us-east-1")
	s.NoError(err)
	s.Equal(.21, price)

	price, err = pkgCachingPriceFetcher.getEC2OnDemandCost(context.Background(), s.m.client, osWindows, "c3.xlarge", "us-east-1")
	s.NoError(err)
	s.Equal(.376, price)

	price, err = pkgCachingPriceFetcher.getEC2OnDemandCost(context.Background(), s.m.client, osLinux, "m5.4xlarge", "us-east-1")
	s.NoError(err)
	s.Equal(.768, price)

	price, err = pkgCachingPriceFetcher.getEC2OnDemandCost(context.Background(), s.m.client, osWindows, "m5.4xlarge", "us-east-1")
	s.NoError(err)
	s.Equal(1.504, price)

	price, err = pkgCachingPriceFetcher.getEC2OnDemandCost(context.Background(), s.m.client, osLinux, "m5.xlarge", "us-east-1")
	s.NoError(err)
	s.Equal(.192, price)

	price, err = pkgCachingPriceFetcher.getEC2OnDemandCost(context.Background(), s.m.client, osWindows, "m5.xlarge", "us-east-1")
	s.NoError(err)
	s.Equal(.376, price)

	s.Equal(.84, pkgCachingPriceFetcher.ec2Prices[odInfo{os: "Linux", instance: "c3.4xlarge", region: "US East (N. Virginia)"}])
	s.Equal(1.504, pkgCachingPriceFetcher.ec2Prices[odInfo{os: "Windows", instance: "c3.4xlarge", region: "US East (N. Virginia)"}])
	s.Equal(.21, pkgCachingPriceFetcher.ec2Prices[odInfo{os: "Linux", instance: "c3.xlarge", region: "US East (N. Virginia)"}])
	s.Equal(.376, pkgCachingPriceFetcher.ec2Prices[odInfo{os: "Windows", instance: "c3.xlarge", region: "US East (N. Virginia)"}])
	s.Equal(.768, pkgCachingPriceFetcher.ec2Prices[odInfo{os: "Linux", instance: "m5.4xlarge", region: "US East (N. Virginia)"}])
	s.Equal(1.504, pkgCachingPriceFetcher.ec2Prices[odInfo{os: "Windows", instance: "m5.4xlarge", region: "US East (N. Virginia)"}])
	s.Equal(.192, pkgCachingPriceFetcher.ec2Prices[odInfo{os: "Linux", instance: "m5.xlarge", region: "US East (N. Virginia)"}])
	s.Equal(.376, pkgCachingPriceFetcher.ec2Prices[odInfo{os: "Windows", instance: "m5.xlarge", region: "US East (N. Virginia)"}])
}
