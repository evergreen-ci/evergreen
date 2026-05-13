package ec2instancereferenceprice

import "github.com/mongodb/anser/bsonutil"

// EC2InstanceReferencePrice holds median on-demand and finance-adjusted hourly
// rates for an instance type and OS, used to validate distro cost_data.
type EC2InstanceReferencePrice struct {
	InstanceType          string  `bson:"instance_type" json:"instance_type"`
	OperatingSystem       string  `bson:"operating_system" json:"operating_system"`
	OnDemandMedian        float64 `bson:"on_demand_median" json:"on_demand_median"`
	AdjustedFinanceMedian float64 `bson:"adjusted_finance_median" json:"adjusted_finance_median"`
}

var (
	// BSON field names for queries (aligned with struct tags).
	InstanceTypeKey    = bsonutil.MustHaveTag(EC2InstanceReferencePrice{}, "InstanceType")
	OperatingSystemKey = bsonutil.MustHaveTag(EC2InstanceReferencePrice{}, "OperatingSystem")
)
