package cost

import "github.com/mongodb/anser/bsonutil"

var (
	OnDemandEC2CostKey               = bsonutil.MustHaveTag(Cost{}, "OnDemandEC2Cost")
	AdjustedEC2CostKey               = bsonutil.MustHaveTag(Cost{}, "AdjustedEC2Cost")
	OnDemandEBSThroughputCostKey     = bsonutil.MustHaveTag(Cost{}, "OnDemandEBSThroughputCost")
	AdjustedEBSThroughputCostKey     = bsonutil.MustHaveTag(Cost{}, "AdjustedEBSThroughputCost")
	OnDemandEBSStorageCostKey        = bsonutil.MustHaveTag(Cost{}, "OnDemandEBSStorageCost")
	AdjustedEBSStorageCostKey        = bsonutil.MustHaveTag(Cost{}, "AdjustedEBSStorageCost")
	OnDemandS3ArtifactPutCostKey     = bsonutil.MustHaveTag(Cost{}, "OnDemandS3ArtifactPutCost")
	AdjustedS3ArtifactPutCostKey     = bsonutil.MustHaveTag(Cost{}, "AdjustedS3ArtifactPutCost")
	OnDemandS3LogPutCostKey          = bsonutil.MustHaveTag(Cost{}, "OnDemandS3LogPutCost")
	AdjustedS3LogPutCostKey          = bsonutil.MustHaveTag(Cost{}, "AdjustedS3LogPutCost")
	OnDemandS3ArtifactStorageCostKey = bsonutil.MustHaveTag(Cost{}, "OnDemandS3ArtifactStorageCost")
	AdjustedS3ArtifactStorageCostKey = bsonutil.MustHaveTag(Cost{}, "AdjustedS3ArtifactStorageCost")
	OnDemandS3LogStorageCostKey      = bsonutil.MustHaveTag(Cost{}, "OnDemandS3LogStorageCost")
	AdjustedS3LogStorageCostKey      = bsonutil.MustHaveTag(Cost{}, "AdjustedS3LogStorageCost")
)

// Cost represents a cost breakdown for tasks and versions
type Cost struct {
	// Total is the sum of adjusted cost components.
	Total float64 `bson:"-" json:"total,omitempty"`
	// OnDemandEC2Cost is the cost calculated using only on-demand rates.
	OnDemandEC2Cost float64 `bson:"on_demand_ec2_cost,omitempty" json:"on_demand_ec2_cost,omitempty"`
	// AdjustedEC2Cost is the cost calculated using the finance formula with savings plan and on-demand components.
	AdjustedEC2Cost float64 `bson:"adjusted_ec2_cost,omitempty" json:"adjusted_ec2_cost,omitempty"`
	// OnDemandEBSThroughputCost is the cost of EBS GP3 throughput calculated using on-demand rates.
	OnDemandEBSThroughputCost float64 `bson:"on_demand_ebs_throughput_cost,omitempty" json:"-"`
	// AdjustedEBSThroughputCost is the adjusted cost of EBS GP3 throughput with discount applied.
	AdjustedEBSThroughputCost float64 `bson:"adjusted_ebs_throughput_cost,omitempty" json:"adjusted_ebs_throughput_cost"`
	// OnDemandEBSStorageCost is the cost of EBS storage calculated using on-demand rates.
	OnDemandEBSStorageCost float64 `bson:"on_demand_ebs_storage_cost,omitempty" json:"-"`
	// AdjustedEBSStorageCost is the adjusted cost of EBS storage (GP3/GP2) with discount applied.
	AdjustedEBSStorageCost float64 `bson:"adjusted_ebs_storage_cost,omitempty" json:"adjusted_ebs_storage_cost"`
	// OnDemandS3ArtifactPutCost is the standard (non-discounted) S3 PUT request cost for uploading user artifacts.
	OnDemandS3ArtifactPutCost float64 `bson:"on_demand_s3_artifact_put_cost,omitempty" json:"-"`
	// AdjustedS3ArtifactPutCost is the adjusted (discounted) S3 PUT request cost for uploading user artifacts.
	AdjustedS3ArtifactPutCost float64 `bson:"adjusted_s3_artifact_put_cost,omitempty" json:"adjusted_s3_artifact_put_cost,omitempty"`
	// OnDemandS3LogPutCost is the standard (non-discounted) S3 PUT request cost for uploading task log chunks.
	OnDemandS3LogPutCost float64 `bson:"on_demand_s3_log_put_cost,omitempty" json:"-"`
	// AdjustedS3LogPutCost is the adjusted (discounted) S3 PUT request cost for uploading task log chunks.
	AdjustedS3LogPutCost float64 `bson:"adjusted_s3_log_put_cost,omitempty" json:"adjusted_s3_log_put_cost,omitempty"`
	// OnDemandS3ArtifactStorageCost is the standard (non-discounted) S3 storage cost for artifact bytes over their retention period.
	OnDemandS3ArtifactStorageCost float64 `bson:"on_demand_s3_artifact_storage_cost,omitempty" json:"-"`
	// AdjustedS3ArtifactStorageCost is the adjusted (discounted) S3 storage cost for artifact bytes over their retention period.
	AdjustedS3ArtifactStorageCost float64 `bson:"adjusted_s3_artifact_storage_cost,omitempty" json:"adjusted_s3_artifact_storage_cost,omitempty"`
	// OnDemandS3LogStorageCost is the standard (non-discounted) S3 storage cost for log bytes over their retention period.
	OnDemandS3LogStorageCost float64 `bson:"on_demand_s3_log_storage_cost,omitempty" json:"-"`
	// AdjustedS3LogStorageCost is the adjusted (discounted) S3 storage cost for log bytes over their retention period.
	AdjustedS3LogStorageCost float64 `bson:"adjusted_s3_log_storage_cost,omitempty" json:"adjusted_s3_log_storage_cost,omitempty"`
}

// TotalAdjusted returns the sum of every adjusted component in this value, excluding on-demand fields,
// which are an alternate pricing view of the same usage.
func (c Cost) TotalAdjusted() float64 {
	return c.AdjustedEC2Cost +
		c.AdjustedEBSThroughputCost +
		c.AdjustedEBSStorageCost +
		c.AdjustedS3ArtifactPutCost +
		c.AdjustedS3LogPutCost +
		c.AdjustedS3ArtifactStorageCost +
		c.AdjustedS3LogStorageCost
}

// IsZero returns true if all cost components are zero.
func (c Cost) IsZero() bool {
	return c.OnDemandEC2Cost == 0 &&
		c.AdjustedEC2Cost == 0 &&
		c.OnDemandEBSThroughputCost == 0 &&
		c.AdjustedEBSThroughputCost == 0 &&
		c.OnDemandEBSStorageCost == 0 &&
		c.AdjustedEBSStorageCost == 0 &&
		c.OnDemandS3ArtifactPutCost == 0 &&
		c.OnDemandS3LogPutCost == 0 &&
		c.OnDemandS3ArtifactStorageCost == 0 &&
		c.OnDemandS3LogStorageCost == 0 &&
		c.AdjustedS3ArtifactPutCost == 0 &&
		c.AdjustedS3LogPutCost == 0 &&
		c.AdjustedS3ArtifactStorageCost == 0 &&
		c.AdjustedS3LogStorageCost == 0
}
