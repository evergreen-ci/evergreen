package cost

// Cost represents a cost breakdown for tasks and versions
type Cost struct {
	// OnDemandEC2Cost is the cost calculated using only on-demand rates.
	OnDemandEC2Cost float64 `bson:"on_demand_ec2_cost,omitempty" json:"on_demand_ec2_cost,omitempty"`
	// AdjustedEC2Cost is the cost calculated using the finance formula with savings plan and on-demand components.
	AdjustedEC2Cost float64 `bson:"adjusted_ec2_cost,omitempty" json:"adjusted_ec2_cost,omitempty"`
	// S3ArtifactPutCost is the calculated S3 PUT request cost for uploading user artifacts.
	S3ArtifactPutCost float64 `bson:"s3_artifact_put_cost,omitempty" json:"s3_artifact_put_cost,omitempty"`
}

// IsZero returns true if all cost components are zero.
func (c Cost) IsZero() bool {
	return c.OnDemandEC2Cost == 0 && c.AdjustedEC2Cost == 0 && c.S3ArtifactPutCost == 0
}
