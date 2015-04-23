package pricing

import (
	"io/ioutil"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	_ "launchpad.net/xmlpath"
)

func mustReadFile(t *testing.T, path string) []byte {
	b, e := ioutil.ReadFile(path)
	if e != nil {
		t.Fatal(e.Error())
	}
	return b
}

func TestLoadPricing(t *testing.T) {
	Convey("loadPricesFor", t, func() {
		prices, e := loadPricesFor("linux-od.json")
		if e != nil {
			t.Fatal(e)
		}
		regions := prices.RegionNames()
		So(len(regions), ShouldEqual, 8)
		if len(regions) < 1 {
			t.Fatal("at least 1 region must be found")
		}
		So(regions[0], ShouldEqual, "us-east")
		if len(prices.Config.Regions) < 1 {
			t.Fatal("at least 1 region must be found")
		}

		region := prices.Config.Regions[0]
		types := region.InstanceTypes
		if len(types) < 1 {
			t.Fatal("no types found")
		}
		So(len(types), ShouldEqual, 10)
		instanceType := types[0]

		size := instanceType.Sizes[0]
		So(size.Size, ShouldEqual, "m3.medium")
		vc := size.ValueColumns[0]
		So(vc.Name, ShouldEqual, "linux")
		So(vc.Prices["USD"], ShouldEqual, "0.113")
	})
}

func TestValueColumnes(t *testing.T) {
	Convey("Value Columns", t, func() {
		Convey("on demand instances", func() {
			vcs := ValueColumns{
				{Name: "linux", Prices: map[string]string{"USD": "0.450"}},
			}
			So(vcs, ShouldNotBeNil)
			So(len(vcs.Prices()), ShouldEqual, 1)

			price := vcs.Prices()[0]
			So(price.PerHour, ShouldEqual, 0.45)
			So(price.TotalPerHour(), ShouldEqual, 0.45)
			So(price.Upfront, ShouldEqual, 0)

		})
		Convey("reserved instances", func() {
			vcs := ValueColumns{
				{Name: "yrTerm1Hourly", Rate: "perhr", Prices: map[string]string{"USD": "0.028"}},
				{Name: "yrTerm1", Prices: map[string]string{"USD": "338"}},
				{Name: "yrTerm3Hourly", Rate: "perhr", Prices: map[string]string{"USD": "0.023"}},
				{Name: "yrTerm3", Prices: map[string]string{"USD": "514"}},
			}
			So(vcs, ShouldNotBeNil)
			So(len(vcs.Prices()), ShouldEqual, 2)

			price := vcs.Prices()[0]
			So(price.Upfront, ShouldEqual, 338)
			So(price.PerHour, ShouldEqual, 0.028)
			So(price.TotalPerHour(), ShouldBeBetween, 0.06658, 0.06659)

			price = vcs.Prices()[1]
			So(price.Upfront, ShouldEqual, 514)
			So(price.PerHour, ShouldEqual, 0.023)
			So(price.TotalPerHour(), ShouldBeBetween, 0.042558, 0.042559)
		})
	})
}
