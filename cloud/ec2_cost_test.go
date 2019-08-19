// +build !race

package cloud

import (
	"context"
	"testing"
	"time"

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

type CostIntegrationSuite struct {
	suite.Suite
	m      *ec2Manager
	client AWSClient
	h      *host.Host
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
	s.h = &host.Host{
		Id: "h1",
		Distro: distro.Distro{
			ProviderSettings: &map[string]interface{}{
				"key_name":           "key",
				"aws_access_key_id":  "key_id",
				"ami":                "ami",
				"instance_type":      "instance",
				"security_group_ids": []string{"abcdef"},
				"bid_price":          float64(0.001),
			},
			Provider: evergreen.ProviderNameEc2OnDemand,
		},
	}
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

func (s *CostIntegrationSuite) TestGetProviderStatic() {
	settings := &EC2ProviderSettings{}
	settings.InstanceType = "m4.large"
	settings.IsVpc = true

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.m.provider = onDemandProvider
	provider, err := s.m.getProvider(ctx, s.h, settings)
	s.NoError(err)
	s.Equal(onDemandProvider, provider)

	s.m.provider = spotProvider
	provider, err = s.m.getProvider(ctx, s.h, settings)
	s.NoError(err)
	s.Equal(spotProvider, provider)

	s.m.provider = 5
	_, err = s.m.getProvider(ctx, s.h, settings)
	s.Error(err)

	s.m.provider = -5
	_, err = s.m.getProvider(ctx, s.h, settings)
	s.Error(err)
}

func (s *CostIntegrationSuite) TestGetProviderAuto() {
	s.h.Distro.Arch = "linux"
	settings := &EC2ProviderSettings{}
	s.m.provider = autoProvider

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m4LargeOnDemand, err := pkgCachingPriceFetcher.getEC2OnDemandCost(context.Background(), s.m.client, getOsName(s.h), "m4.large", defaultRegion)
	s.InDelta(.1, m4LargeOnDemand, .05)
	s.NoError(err)

	t2MicroOnDemand, err := pkgCachingPriceFetcher.getEC2OnDemandCost(context.Background(), s.m.client, getOsName(s.h), "t2.micro", defaultRegion)
	s.InDelta(.0116, t2MicroOnDemand, .01)
	s.NoError(err)

	settings.InstanceType = "m4.large"
	settings.IsVpc = true
	m4LargeSpot, az, err := pkgCachingPriceFetcher.getLatestLowestSpotCostForInstance(ctx, s.m.client, settings, getOsName(s.h))
	s.Contains(az, "us-east")
	s.True(m4LargeSpot > 0)
	s.NoError(err)

	settings.InstanceType = "t2.micro"
	settings.IsVpc = true
	t2MicroSpot, az, err := pkgCachingPriceFetcher.getLatestLowestSpotCostForInstance(ctx, s.m.client, settings, getOsName(s.h))
	s.Contains(az, "us-east")
	s.True(t2MicroSpot > 0)
	s.NoError(err)

	settings.InstanceType = "m4.large"
	settings.IsVpc = true
	provider, err := s.m.getProvider(ctx, s.h, settings)
	s.NoError(err)
	if m4LargeSpot < m4LargeOnDemand {
		s.Equal(spotProvider, provider)
		s.Equal(evergreen.ProviderNameEc2Spot, s.h.Distro.Provider)
	} else {
		s.Equal(onDemandProvider, provider)
		s.Equal(evergreen.ProviderNameEc2OnDemand, s.h.Distro.Provider)
	}

	settings.InstanceType = "t2.micro"
	settings.IsVpc = true
	provider, err = s.m.getProvider(ctx, s.h, settings)
	s.NoError(err)
	if t2MicroSpot < t2MicroOnDemand {
		s.Equal(spotProvider, provider)
		s.Equal(evergreen.ProviderNameEc2Spot, s.h.Distro.Provider)
	} else {
		s.Equal(onDemandProvider, provider)
		s.Equal(evergreen.ProviderNameEc2OnDemand, s.h.Distro.Provider)
	}
}
