package ec2

import (
	"fmt"
	"sort"

	"github.com/dynport/gocli"
	"github.com/dynport/gocloud/aws/pricing"
)

type Prices struct {
	Region   string `cli:"type=opt short=r default=eu-ireland"`
	Heavy    bool   `cli:"type=opt long=heavy"`
	Detailed bool   `cli:"type=opt long=detailed"`
}

func (a *Prices) Run() error {
	configs, e := pricing.AllInstanceTypeConfigs()
	if e != nil {
		return e
	}
	sort.Sort(configs)
	var pr *pricing.Pricing
	regionName := a.Region
	typ := "od"
	if a.Heavy {
		regionName = normalizeRegion(regionName)
		typ = "ri-heavy"
		pr, e = pricing.LinuxReservedHeavy()
	} else {
		regionName = normalizeRegionForOd(regionName)
		pr, e = pricing.LinuxOnDemand()
	}
	if e != nil {
		return e
	}
	priceMapping := map[string]pricing.PriceList{}
	region := pr.FindRegion(regionName)
	if region == nil {
		return fmt.Errorf("could not find prices for reagion %q. Known regions are %v", regionName, pr.RegionNames())
	}
	for _, t := range region.InstanceTypes {
		for _, size := range t.Sizes {
			priceMapping[size.Size] = size.ValueColumns.Prices()
		}
	}
	if a.Detailed {
		printConfigsDetailed(regionName, typ, priceMapping, configs)
	} else {
		printConfigs(regionName, typ, priceMapping, configs)
	}
	return nil
}

func printConfigsDetailed(regionName string, typ string, priceMapping map[string]pricing.PriceList, configs pricing.InstanceTypeConfigs) {
	table := gocli.NewTable()
	for _, config := range configs {
		table.Add("Type", config.Name)
		table.Add("CPUs", config.Cpus)
		table.Add("Storage", config.Storage)
		table.Add("-------", "")
	}
	fmt.Println(table)
}

func printConfigs(regionName string, typ string, priceMapping map[string]pricing.PriceList, configs pricing.InstanceTypeConfigs) {
	table := gocli.NewTable()
	table.Add("Type", "Cores", "GB RAM", "Region", "Type", "$/Hour", "$/Month", "$/Core", "$/GB")
	for _, config := range configs {
		cols := []interface{}{
			config.Name, config.Cpus, config.Memory,
		}
		if prices, ok := priceMapping[config.Name]; ok {
			cols = append(cols, normalizeRegion(regionName), typ)
			if len(prices) > 0 {
				sort.Sort(prices)
				price := prices[0].TotalPerHour()
				perMonth := price * HOURS_PER_MONTH
				perCore := perMonth / float64(config.Cpus)
				perGb := perMonth / config.Memory
				cols = append(cols, fmt.Sprintf("%.03f", price), monthlyPrice(perMonth), monthlyPrice(perCore), monthlyPrice(perGb))
			}
		}
		table.Add(cols...)
	}
	fmt.Println(table)
}
