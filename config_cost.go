package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	// S3PutRequestCost is the cost per S3 PUT request ($0.000005).
	S3PutRequestCost = 0.000005
)

// CostConfig represents the admin config section for finance-related settings.
type CostConfig struct {
	// FinanceFormula determines the weighting/percentage of the two parts of total cost: savingsPlanComponent and onDemandComponent.
	FinanceFormula float64 `bson:"finance_formula" json:"finance_formula" yaml:"finance_formula"`
	// SavingsPlanDiscount is the discount rate (0.0-1.0) applied to savings plan pricing
	SavingsPlanDiscount float64 `bson:"savings_plan_discount" json:"savings_plan_discount" yaml:"savings_plan_discount"`
	// OnDemandDiscount is the discount rate (0.0-1.0) applied to on-demand pricing
	OnDemandDiscount float64 `bson:"on_demand_discount" json:"on_demand_discount" yaml:"on_demand_discount"`
	// S3Cost holds S3-related cost discount configuration
	S3Cost S3CostConfig `bson:"s3_cost" json:"s3_cost" yaml:"s3_cost"`
}

// S3UploadCostConfig represents S3 upload cost discount configuration.
type S3UploadCostConfig struct {
	UploadCostDiscount float64 `bson:"upload_cost_discount" json:"upload_cost_discount" yaml:"upload_cost_discount"`
}

// S3StorageCostConfig represents S3 storage cost discount configuration.
type S3StorageCostConfig struct {
	StandardStorageCostDiscount float64 `bson:"standard_storage_cost_discount" json:"standard_storage_cost_discount" yaml:"standard_storage_cost_discount"`
	IAStorageCostDiscount       float64 `bson:"i_a_storage_cost_discount" json:"i_a_storage_cost_discount" yaml:"i_a_storage_cost_discount"`
}

// S3CostConfig represents S3 cost configuration with separate upload and storage settings.
type S3CostConfig struct {
	Upload  S3UploadCostConfig  `bson:"upload" json:"upload" yaml:"upload"`
	Storage S3StorageCostConfig `bson:"storage" json:"storage" yaml:"storage"`
}

var (
	financeConfigFormulaKey             = bsonutil.MustHaveTag(CostConfig{}, "FinanceFormula")
	financeConfigSavingsPlanDiscountKey = bsonutil.MustHaveTag(CostConfig{}, "SavingsPlanDiscount")
	financeConfigOnDemandDiscountKey    = bsonutil.MustHaveTag(CostConfig{}, "OnDemandDiscount")
	financeConfigS3CostKey              = bsonutil.MustHaveTag(CostConfig{}, "S3Cost")
)

func (*CostConfig) SectionId() string { return "cost" }

func (c *CostConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *CostConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			financeConfigFormulaKey:             c.FinanceFormula,
			financeConfigSavingsPlanDiscountKey: c.SavingsPlanDiscount,
			financeConfigOnDemandDiscountKey:    c.OnDemandDiscount,
			financeConfigS3CostKey:              c.S3Cost,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func validateDiscountField(value float64, fieldName string, catcher grip.Catcher) {
	if value < 0.0 || value > 1.0 {
		catcher.Errorf("%s must be between 0.0 and 1.0", fieldName)
	}
}

func (c *CostConfig) ValidateAndDefault() error {
	catcher := grip.NewBasicCatcher()

	validateDiscountField(c.FinanceFormula, "finance formula", catcher)
	validateDiscountField(c.SavingsPlanDiscount, "savings plan discount", catcher)
	validateDiscountField(c.OnDemandDiscount, "on demand discount", catcher)
	validateDiscountField(c.S3Cost.Upload.UploadCostDiscount, "S3 upload cost discount", catcher)
	validateDiscountField(c.S3Cost.Storage.StandardStorageCostDiscount, "S3 standard storage cost discount", catcher)
	validateDiscountField(c.S3Cost.Storage.IAStorageCostDiscount, "S3 infrequent access storage cost discount", catcher)

	return catcher.Resolve()
}

// IsConfigured returns true if any finance config field is set.
func (c *CostConfig) IsConfigured() bool {
	return c.FinanceFormula != 0 ||
		c.SavingsPlanDiscount != 0 ||
		c.OnDemandDiscount != 0 ||
		c.S3Cost.Upload.UploadCostDiscount != 0 ||
		c.S3Cost.Storage.StandardStorageCostDiscount != 0 ||
		c.S3Cost.Storage.IAStorageCostDiscount != 0
}
