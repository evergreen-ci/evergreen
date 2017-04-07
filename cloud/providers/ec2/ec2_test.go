package ec2

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
)

var testConfig = testutil.TestConfig()

// mins returns a time X minutes after UNIX epoch
func mins(x int64) time.Time {
	return time.Unix(60*x, 0)
}

func TestCostForRange(t *testing.T) {
	Convey("With 5 hours of test host billing", t, func() {
		rates := []spotRate{
			{Time: mins(0), Price: 1.0},
			{Time: mins(60), Price: .5},
			{Time: mins(2 * 60), Price: 1.0},
			{Time: mins(3 * 60), Price: 2.0},
			{Time: mins(4 * 60), Price: 1.0},
		}
		Convey("and a 30-min task that ran within the first hour", func() {
			price := spotCostForRange(mins(10), mins(40), rates)
			Convey("should cost .5", func() {
				So(price, ShouldEqual, .5)
			})
		})
		Convey("and a 30-min task that ran within the last hour", func() {
			price := spotCostForRange(mins(10+4*60), mins(40+4*60), rates)
			Convey("should cost .5", func() {
				So(price, ShouldEqual, .5)
			})
		})
		Convey("and a 30-min task that ran between the first and second hour", func() {
			price := spotCostForRange(mins(45), mins(75), rates)
			Convey("should cost .25 + .125", func() {
				So(price, ShouldEqual, .375)
			})
		})
		Convey("and a 120-min task that ran between the first three hours", func() {
			price := spotCostForRange(mins(30), mins(150), rates)
			Convey("should cost .5 + .5 + .5", func() {
				So(price, ShouldEqual, 1.5)
			})
		})
		Convey("and an 30-min task started after the last reported time", func() {
			price := spotCostForRange(mins(4*60+30), mins(5*60), rates)
			Convey("should cost .5", func() {
				So(price, ShouldEqual, .5)
			})
		})
		Convey("and an earlier 30-min task started after the last reported time", func() {
			price := spotCostForRange(mins(4*60), mins(5*60), rates)
			Convey("should cost 1", func() {
				So(price, ShouldEqual, 1)
			})
		})
		Convey("and a task that starts at the same time as the first bucket", func() {
			price := spotCostForRange(mins(0), mins(30), rates)
			Convey("should still report a proper time", func() {
				So(price, ShouldEqual, .5)
			})
		})
		Convey("and a task that starts at before the first bucket", func() {
			price := spotCostForRange(mins(-60), mins(45), rates)
			Convey("should compute based on the first bucket", func() {
				So(price, ShouldEqual, 1.75)
			})
		})
	})
}

func TestSpotPriceHistory(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestSpotPriceHistory")
	Convey("With a Spot Manager", t, func() {
		m := &EC2SpotManager{}
		testutil.HandleTestingErr(m.Configure(testConfig), t, "failed to configure spot manager")
		Convey("loading 2 hours of price history should succeed", func() {
			ps, err := m.describeHourlySpotPriceHistory("m3.large", "us-east-1a", osLinux,
				time.Now().Add(-2*time.Hour), time.Now())
			So(err, ShouldBeNil)
			So(len(ps), ShouldBeGreaterThan, 2)
			Convey("and the results should be sane", func() {
				So(ps[len(ps)-1].Time, ShouldHappenBetween,
					time.Now().Add(-10*time.Minute), time.Now())
				So(ps[0].Price, ShouldBeBetween, 0.0, 2.0)
				So(ps[0].Time, ShouldHappenBefore, ps[1].Time)
			})
		})
		Convey("loading 10 days of price history should succeed", func() {
			ps, err := m.describeHourlySpotPriceHistory("m3.large", "us-east-1a", osLinux,
				time.Now().Add(-240*time.Hour), time.Now())
			So(err, ShouldBeNil)
			So(len(ps), ShouldBeGreaterThan, 240)
			Convey("and the results should be sane", func() {
				So(ps[len(ps)-1].Time, ShouldHappenBetween, time.Now().Add(-30*time.Minute), time.Now())
				So(ps[0].Time, ShouldHappenWithin,
					time.Hour, time.Now().Add(-241*time.Hour))
				So(ps[0].Price, ShouldBeBetween, 0.0, 2.0)
				So(ps[0].Time, ShouldHappenBefore, ps[1].Time)
			})
		})
	})
}

func TestFetchEBSPricing(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestFetchEBSPricing")
	Convey("Fetching the map of EBS pricing should succeed", t, func() {
		prices, err := fetchEBSPricing()
		So(err, ShouldBeNil)
		Convey("and the resulting map should be sane", func() {
			So(len(prices), ShouldBeGreaterThan, 5)
			So(prices["us-east-1"], ShouldBeBetween, 0, 1)
		})
	})
}

type mockEBSPriceFetcher struct {
	response map[string]float64
	err      error
}

func (mpf mockEBSPriceFetcher) FetchEBSPrices() (map[string]float64, error) {
	if mpf.err != nil {
		return nil, mpf.err
	}
	return mpf.response, nil
}

func TestEBSCostCalculation(t *testing.T) {
	Convey("With a price of $1.00/GB-Month", t, func() {
		region := "X"
		pf := mockEBSPriceFetcher{
			response: map[string]float64{
				region: 1.00,
			},
		}
		Convey("a 1-GB drive for 1 month should cost $1", func() {
			cost, err := ebsCost(pf, region, 1, time.Hour*24*30)
			So(err, ShouldBeNil)
			So(cost, ShouldEqual, 1.00)
		})
		Convey("a 20-GB drive for 1 month should cost $20", func() {
			cost, err := ebsCost(pf, region, 20, time.Hour*24*30)
			So(err, ShouldBeNil)
			So(cost, ShouldEqual, 20)
		})
		Convey("a 100-GB drive for 1 hour should cost around $0.14", func() {
			cost, err := ebsCost(pf, region, 100, time.Hour)
			So(err, ShouldBeNil)
			So(cost, ShouldBeBetween, 0.13, 0.14)
		})
		Convey("a 100-GB drive for 20 mins should cost around $0.04", func() {
			cost, err := ebsCost(pf, region, 100, time.Minute*20)
			So(err, ShouldBeNil)
			So(cost, ShouldBeBetween, 0.04, 0.05)
		})
	})

	Convey("With erroring price fetchers", t, func() {
		Convey("a network error should bubble up", func() {
			pf := mockEBSPriceFetcher{err: errors.New("NETWORK OH NO")}
			_, err := ebsCost(pf, "", 100, time.Minute*20)
			So(err, ShouldNotBeNil)
		})
		Convey("a made-up region should return an error", func() {
			pf := mockEBSPriceFetcher{response: map[string]float64{}}
			_, err := ebsCost(pf, "mars-west-1", 100, time.Minute*20)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestEBSPriceCaching(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestEBSPriceCaching")
	Convey("With an empty cachedEBSPriceFetcher", t, func() {
		pf := cachedEBSPriceFetcher{}
		So(pf.prices, ShouldBeNil)
		Convey("running FetchEBSPrices should return a map and cache it", func() {
			prices, err := pf.FetchEBSPrices()
			So(err, ShouldBeNil)
			So(prices, ShouldNotBeNil)
			So(prices, ShouldResemble, pf.prices)
			Convey("but a cache should not change if we call fetch again", func() {
				pf.m.Lock()
				pf.prices["NEW"] = 1
				pf.m.Unlock()
				prices, err := pf.FetchEBSPrices()
				So(err, ShouldBeNil)
				So(prices, ShouldNotBeNil)
				So(prices["NEW"], ShouldEqual, 1.0)
			})
		})
	})
}

func TestOnDemandPriceAPITranslation(t *testing.T) {
	Convey("With a set of OS types", t, func() {
		Convey("Linux/UNIX should become Linux", func() {
			So(osBillingName(osLinux), ShouldEqual, "Linux")
		})
		Convey("other OSes should stay the same", func() {
			So(osBillingName(osSUSE), ShouldEqual, string(osSUSE))
			So(osBillingName(osWindows), ShouldEqual, string(osWindows))
		})
	})

	Convey("With a set of region names", t, func() {
		Convey("the full region name should be returned", func() {
			r, err := regionFullname("us-east-1")
			So(err, ShouldBeNil)
			So(r, ShouldEqual, "US East (N. Virginia)")
			r, err = regionFullname("us-west-1")
			So(err, ShouldBeNil)
			So(r, ShouldEqual, "US West (N. California)")
			r, err = regionFullname("us-west-2")
			So(err, ShouldBeNil)
			So(r, ShouldEqual, "US West (Oregon)")
			Convey("but an unknown region will return an error", func() {
				r, err = regionFullname("amazing")
				So(err, ShouldNotBeNil)
			})
		})
	})
}

type mockODPriceFetcher struct {
	price float64
	err   error
}

func (mpf *mockODPriceFetcher) FetchPrice(_ osType, _, _ string) (float64, error) {
	if mpf.err != nil {
		return 0, mpf.err
	}
	return mpf.price, nil
}

func TestOnDemandPriceCalculation(t *testing.T) {
	Convey("With prices of $1.00/hr", t, func() {
		pf := &mockODPriceFetcher{1.0, nil}
		Convey("a half-hour task should cost 50¢", func() {
			cost, err := onDemandCost(pf, osLinux, "m3.4xlarge", "us-east-1", time.Minute*30)
			So(err, ShouldBeNil)
			So(cost, ShouldEqual, .50)
		})
		Convey("an hour task should cost $1", func() {
			cost, err := onDemandCost(pf, osLinux, "m3.4xlarge", "us-east-1", time.Hour)
			So(err, ShouldBeNil)
			So(cost, ShouldEqual, 1)
		})
		Convey("a two-hour task should cost $2", func() {
			cost, err := onDemandCost(pf, osLinux, "m3.4xlarge", "us-east-1", time.Hour*2)
			So(err, ShouldBeNil)
			So(cost, ShouldEqual, 2)
		})
	})
	Convey("With prices of $0.00/hr", t, func() {
		pf := &mockODPriceFetcher{0, nil}
		Convey("onDemandPrice should return a 'not found' error", func() {
			cost, err := onDemandCost(pf, osLinux, "m3.4xlarge", "us-east-1", time.Hour)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "not found")
			So(cost, ShouldEqual, 0)
		})
	})
	Convey("With an erroring PriceFetcher", t, func() {
		pf := &mockODPriceFetcher{1, errors.New("bad thing")}
		Convey("errors should be bubbled up", func() {
			cost, err := onDemandCost(pf, osLinux, "m3.4xlarge", "us-east-1", time.Hour*2)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "bad thing")
			So(cost, ShouldEqual, 0)
		})
	})
}

func TestFetchOnDemandPricing(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestOnDemandPriceCaching")
	Convey("With an empty cachedOnDemandPriceFetcher", t, func() {
		pf := cachedOnDemandPriceFetcher{}
		So(pf.prices, ShouldBeNil)
		Convey("various prices in us-east-1 should be sane", func() {
			c34x, err := pf.FetchPrice(osLinux, "c3.4xlarge", "us-east-1")
			So(err, ShouldBeNil)
			So(c34x, ShouldBeGreaterThan, .80)
			c3x, err := pf.FetchPrice(osLinux, "c3.xlarge", "us-east-1")
			So(err, ShouldBeNil)
			So(c3x, ShouldBeGreaterThan, .20)
			So(c34x, ShouldBeGreaterThan, c3x)
			wc3x, err := pf.FetchPrice(osWindows, "c3.xlarge", "us-east-1")
			So(err, ShouldBeNil)
			So(wc3x, ShouldBeGreaterThan, .20)
			So(wc3x, ShouldBeGreaterThan, c3x)

			Convey("and prices should be cached", func() {
				So(len(pf.prices), ShouldBeGreaterThan, 50)
			})
		})
	})
}

/* This is an example of how the cost calculation functions work.
   This function can be uncommented to manually play with
func TestCostForDuration(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestSpotPriceHistory")
	m := &EC2Manager{}
	m.Configure(testConfig)
	h := &host.Host{Id: "i-359e91ac"}
	h.Distro.Arch = "windows_amd64"
	layout := "Jan 2, 2006 3:04:05 pm -0700"
	start, err := time.Parse(layout, "Sep 8, 2016 11:00:22 am -0400")
	if err != nil {
		panic(err)
	}
	fmt.Println(start)
	end, err := time.Parse(layout, "Sep 8, 2016 12:00:49 pm -0400")
	if err != nil {
		panic(err)
	}
	cost, err := m.CostForDuration(h, start, end)
	if err != nil {
		panic(err)
	}
	fmt.Println("PRICE", cost)
	cost, err = m.CostForDuration(h, start, end)
	if err != nil {
		panic(err)
	}
	fmt.Println("PRICE AGAIN", cost)
}*/
