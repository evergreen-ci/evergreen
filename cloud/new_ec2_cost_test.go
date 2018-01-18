package cloud

import (
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
	h := &host.Host{}
	h.Distro.Provider = evergreen.ProviderNameEc2OnDemandNew
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
	now := time.Now()
	cost, err := cpf.getEC2Cost(client, h, timeRange{now.Add(-30 * time.Minute), now})
	s.NoError(err)
	s.Equal(.50, cost)
	cost, err = cpf.getEC2Cost(client, h, timeRange{now.Add(-time.Hour), now})
	s.NoError(err)
	s.Equal(1.0, cost)
	cost, err = cpf.getEC2Cost(client, h, timeRange{now.Add(-2 * time.Hour), now})
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
}

func TestCostIntegrationSuite(t *testing.T) {
	suite.Run(t, new(CostIntegrationSuite))
}

func (s *CostIntegrationSuite) SetupSuite() {
	settings := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(s.T(), settings, "CostIntegrationSuite")
	m := NewEC2Manager(&EC2ManagerOptions{client: &awsClientImpl{}})
	s.m = m.(*ec2Manager)
	s.NoError(s.m.Configure(settings))
	s.NoError(s.m.client.Create(s.m.credentials))
	s.client = s.m.client
}

func (s *CostIntegrationSuite) TestSpotPriceHistory() {
	cpf := cachingPriceFetcher{}
	input := hourlySpotPriceHistoryInput{
		iType: "m3.large",
		zone:  "us-east-1a",
		os:    osLinux,
		start: time.Now().Add(-2 * time.Hour),
		end:   time.Now(),
	}
	ps, err := cpf.describeHourlySpotPriceHistory(s.client, input)
	s.NoError(err)
	s.True(len(ps) > 2)
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
	ps, err = cpf.describeHourlySpotPriceHistory(s.client, input)
	s.NoError(err)
	s.True(len(ps) > 240)
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

func (s *CostIntegrationSuite) TestFetchOnDemandPricing() {
	cpf := cachingPriceFetcher{}
	s.Nil(cpf.ec2Prices)
	c34x, err := cpf.getEC2OnDemandCost(osLinux, "c3.4xlarge", "us-east-1")
	s.NoError(err)
	s.True(c34x > .80)
	c3x, err := cpf.getEC2OnDemandCost(osLinux, "c3.xlarge", "us-east-1")
	s.NoError(err)
	s.True(c3x > .20)
	s.True(c34x > c3x)
	wc3x, err := cpf.getEC2OnDemandCost(osWindows, "c3.xlarge", "us-east-1")
	s.NoError(err)
	s.True(wc3x > .20)
	s.True(wc3x > c3x)
	s.True(len(cpf.ec2Prices) > 50)
}

func (s *CostIntegrationSuite) TestGetProviderStatic() {
	h := &host.Host{}
	settings := &NewEC2ProviderSettings{}

	s.m.provider = onDemandProvider
	provider, err := s.m.getProvider(h, settings)
	s.NoError(err)
	s.Equal(onDemandProvider, provider)

	s.m.provider = spotProvider
	provider, err = s.m.getProvider(h, settings)
	s.NoError(err)
	s.Equal(spotProvider, provider)

	s.m.provider = 5
	_, err = s.m.getProvider(h, settings)
	s.Error(err)

	s.m.provider = -5
	_, err = s.m.getProvider(h, settings)
	s.Error(err)
}

func (s *CostIntegrationSuite) TestGetProviderAuto() {
	h := &host.Host{
		Distro: distro.Distro{
			Arch: "linux",
		},
	}
	settings := &NewEC2ProviderSettings{}
	s.m.provider = autoProvider

	m4LargeOnDemand, err := pkgCachingPriceFetcher.getEC2OnDemandCost(getOsName(h), "m4.large", defaultRegion)
	s.InDelta(.1, m4LargeOnDemand, .05)
	s.NoError(err)

	t2MicroOnDemand, err := pkgCachingPriceFetcher.getEC2OnDemandCost(getOsName(h), "t2.micro", defaultRegion)
	s.InDelta(.0116, t2MicroOnDemand, .01)
	s.NoError(err)

	t1MicroOnDemand, err := pkgCachingPriceFetcher.getEC2OnDemandCost(getOsName(h), "t1.micro", defaultRegion)
	s.InDelta(.0116, t1MicroOnDemand, .01)
	s.NoError(err)

	settings.InstanceType = "m4.large"
	settings.IsVpc = true
	m4LargeSpot, az, err := pkgCachingPriceFetcher.getLatestLowestSpotCostForInstance(s.m.client, settings, getOsName(h))
	s.Contains(az, "us-east")
	s.True(m4LargeSpot > 0)
	s.NoError(err)

	settings.InstanceType = "t2.micro"
	settings.IsVpc = true
	t2MicroSpot, az, err := pkgCachingPriceFetcher.getLatestLowestSpotCostForInstance(s.m.client, settings, getOsName(h))
	s.Contains(az, "us-east")
	s.True(t2MicroSpot > 0)
	s.NoError(err)

	settings.InstanceType = "t1.micro"
	settings.IsVpc = false
	t1MicroSpot, az, err := pkgCachingPriceFetcher.getLatestLowestSpotCostForInstance(s.m.client, settings, getOsName(h))
	s.Contains(az, "us-east")
	s.True(t1MicroSpot > 0)
	s.NoError(err)

	settings.InstanceType = "m4.large"
	settings.IsVpc = true
	provider, err := s.m.getProvider(h, settings)
	s.NoError(err)
	if m4LargeSpot < m4LargeOnDemand {
		s.Equal(spotProvider, provider)
		s.Equal(evergreen.ProviderNameEc2SpotNew, h.Distro.Provider)
	} else {
		s.Equal(onDemandProvider, provider)
		s.Equal(evergreen.ProviderNameEc2OnDemandNew, h.Distro.Provider)
	}

	settings.InstanceType = "t2.micro"
	settings.IsVpc = true
	provider, err = s.m.getProvider(h, settings)
	s.NoError(err)
	if t2MicroSpot < t2MicroOnDemand {
		s.Equal(spotProvider, provider)
		s.Equal(evergreen.ProviderNameEc2SpotNew, h.Distro.Provider)
	} else {
		s.Equal(onDemandProvider, provider)
		s.Equal(evergreen.ProviderNameEc2OnDemandNew, h.Distro.Provider)
	}

	settings.InstanceType = "t1.micro"
	settings.IsVpc = false
	provider, err = s.m.getProvider(h, settings)
	s.NoError(err)
	if t1MicroSpot < t1MicroOnDemand {
		s.Equal(spotProvider, provider)
		s.Equal(evergreen.ProviderNameEc2SpotNew, h.Distro.Provider)
	} else {
		s.Equal(onDemandProvider, provider)
		s.Equal(evergreen.ProviderNameEc2OnDemandNew, h.Distro.Provider)
	}
}
