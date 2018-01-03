package cloud

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/pkg/errors"
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
	pf := mockEBSPriceFetcher{
		response: map[string]float64{
			region: 1.00,
		},
	}
	cost, err := ebsCost(pf, region, 1, time.Hour*24*30)
	s.NoError(err)
	s.Equal(1.00, cost)
	cost, err = ebsCost(pf, region, 20, time.Hour*24*30)
	s.NoError(err)
	s.Equal(20.0, cost)
	cost, err = ebsCost(pf, region, 100, time.Hour)
	s.NoError(err)
	s.InDelta(.135, cost, .005)
	cost, err = ebsCost(pf, region, 100, time.Minute*20)
	s.NoError(err)
	s.InDelta(.045, cost, .005)

	pf = mockEBSPriceFetcher{err: errors.New("NETWORK OH NO")}
	_, err = ebsCost(pf, "", 100, time.Minute*20)
	s.Error(err)
	pf = mockEBSPriceFetcher{response: map[string]float64{}}
	_, err = ebsCost(pf, "mars-west-1", 100, time.Minute*20)
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
	r, err = regionFullname("amazing")
	s.Error(err)
}

func (s *CostUnitSuite) TestOnDemandPriceCalculation() {
	pf := &mockODPriceFetcher{1.0, nil}
	cost, err := onDemandCost(pf, osLinux, "m3.4xlarge", "us-east-1", time.Minute*30)
	s.NoError(err)
	s.Equal(.50, cost)
	cost, err = onDemandCost(pf, osLinux, "m3.4xlarge", "us-east-1", time.Hour)
	s.NoError(err)
	s.Equal(1.0, cost)
	cost, err = onDemandCost(pf, osLinux, "m3.4xlarge", "us-east-1", time.Hour*2)
	s.NoError(err)
	s.Equal(2.0, cost)
	pf = &mockODPriceFetcher{0, nil}
	cost, err = onDemandCost(pf, osLinux, "m3.4xlarge", "us-east-1", time.Hour)
	s.Error(err)
	s.EqualError(err, "price not found in EC2 price listings")
	s.Equal(0.0, cost)
	pf = &mockODPriceFetcher{1, errors.New("bad thing")}
	cost, err = onDemandCost(pf, osLinux, "m3.4xlarge", "us-east-1", time.Hour*2)
	s.Error(err)
	s.EqualError(err, "bad thing")
	s.Equal(0.0, cost)
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
	m *ec2Manager
}

func TestCostIntegrationSuite(t *testing.T) {
	suite.Run(t, new(CostIntegrationSuite))
}

func (s *CostIntegrationSuite) SetupSuite() {
	testutil.ConfigureIntegrationTest(s.T(), testutil.TestConfig(), "CostIntegrationSuite")
	opts := &EC2ManagerOptions{
		client: &awsClientImpl{},
	}
	manager := NewEC2Manager(opts)
	var ok bool
	s.m, ok = manager.(*ec2Manager)
	s.Require().True(ok)
}

func (s *CostIntegrationSuite) TestSpotPriceHistory() {
	ps, err := s.m.describeHourlySpotPriceHistory(hourlySpotPriceHistoryInput{"m3.large", "us-east-1a", osLinux, time.Now().Add(-2 * time.Hour), time.Now()})
	s.NoError(err)
	s.True(len(ps) > 2)
	s.True(ps[len(ps)-1].Time.Before(time.Now()))
	s.True(ps[len(ps)-1].Time.After(time.Now().Add(-10 * time.Minute)))
	s.True(ps[0].Price > 0.0)
	s.True(ps[0].Price < 2.0)
	s.True(ps[0].Time.Before(ps[1].Time))

	ps, err = s.m.describeHourlySpotPriceHistory(hourlySpotPriceHistoryInput{"m3.large", "us-east-1a", osLinux, time.Now().Add(-240 * time.Hour), time.Now()})
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
	prices, err := fetchEBSPricing()
	s.NoError(err)
	s.True(len(prices) > 5)
	s.True(prices["us-east-1"] > 0)
	s.True(prices["us-east-1"] < 1)
}

func (s *CostIntegrationSuite) TestEBSPriceCaching() {
	pf := cachedEBSPriceFetcher{}
	s.Nil(pf.prices)
	prices, err := pf.FetchEBSPrices()
	s.NoError(err)
	s.NotNil(prices)
	s.Equal(pf.prices, prices)
	pf.m.Lock()
	pf.prices["NEW"] = 1
	pf.m.Unlock()
	prices, err = pf.FetchEBSPrices()
	s.NoError(err)
	s.NotNil(prices)
	s.Equal(1.0, prices["NEW"])

}

func (s *CostIntegrationSuite) TestFetchOnDemandPricing() {
	pf := cachedOnDemandPriceFetcher{}
	s.Nil(pf.prices)
	c34x, err := pf.FetchPrice(osLinux, "c3.4xlarge", "us-east-1")
	s.NoError(err)
	s.True(c34x > .80)
	c3x, err := pf.FetchPrice(osLinux, "c3.xlarge", "us-east-1")
	s.NoError(err)
	s.True(c3x > .20)
	s.True(c34x > c3x)
	wc3x, err := pf.FetchPrice(osWindows, "c3.xlarge", "us-east-1")
	s.NoError(err)
	s.True(wc3x > .20)
	s.True(wc3x > c3x)
	s.True(len(pf.prices) > 50)
}
