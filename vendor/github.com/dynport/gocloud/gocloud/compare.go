package main

import (
	"fmt"
	"github.com/dynport/gocli"
	aws "github.com/dynport/gocloud/aws/pricing"
	"github.com/dynport/gocloud/digitalocean"
	_ "github.com/dynport/gocloud/jiffybox"
	_ "github.com/dynport/gocloud/profitbricks"
)

func init() {
	router.RegisterFunc("compare", Compare, "Compare Cloud Providers")
}

func Compare() error {
	table := gocli.NewTable()
	for _, plan := range digitalocean.Plans {
		table.Add(plan.Cores, fmt.Sprintf("%.1f GB", float64(plan.MemoryInMB)/1024.0))
	}

	pricing, e := aws.LinuxOnDemand()
	if e != nil {
		return e
	}
	counts := map[string]int{}
	for _, region := range pricing.Config.Regions {
		for _, typ := range region.InstanceTypes {
			for _, size := range typ.Sizes {
				for _, vc := range size.ValueColumns {
					counts[size.Size]++
					table.Add(region.Region, typ.Type, size.Size, vc.Name, vc.Prices["USD"])
				}
			}
		}
	}
	fmt.Println(table)
	fmt.Println(counts)
	return nil
}
