package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// CostConfig represents the admin config section for finance-related settings.
type CostConfig struct {
	// FinanceFormula determines the weighting/percentage of the two parts of total cost: savingsPlanComponent and onDemandComponent.
	FinanceFormula float64 `bson:"finance_formula" json:"finance_formula" yaml:"finance_formula"`
	// SavingsPlanDiscount is the discount rate (0.0-1.0) applied to savings plan pricing
	SavingsPlanDiscount float64 `bson:"savings_plan_discount" json:"savings_plan_discount" yaml:"savings_plan_discount"`
	// OnDemandDiscount is the discount rate (0.0-1.0) applied to on-demand pricing
	OnDemandDiscount float64 `bson:"on_demand_discount" json:"on_demand_discount" yaml:"on_demand_discount"`
}

var (
	financeConfigFormulaKey             = bsonutil.MustHaveTag(CostConfig{}, "FinanceFormula")
	financeConfigSavingsPlanDiscountKey = bsonutil.MustHaveTag(CostConfig{}, "SavingsPlanDiscount")
	financeConfigOnDemandDiscountKey    = bsonutil.MustHaveTag(CostConfig{}, "OnDemandDiscount")
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
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *CostConfig) ValidateAndDefault() error {
	catcher := grip.NewBasicCatcher()

	if c.FinanceFormula < 0.0 || c.FinanceFormula > 1.0 {
		catcher.Add(errors.New("finance formula must be between 0.0 and 1.0"))
	}
	if c.SavingsPlanDiscount < 0.0 || c.SavingsPlanDiscount > 1.0 {
		catcher.Add(errors.New("savings plan discount must be between 0.0 and 1.0"))
	}
	if c.OnDemandDiscount < 0.0 || c.OnDemandDiscount > 1.0 {
		catcher.Add(errors.New("on demand discount must be between 0.0 and 1.0"))
	}

	return catcher.Resolve()
}

// IsConfigured returns true if any finance config field is set.
func (c *CostConfig) IsConfigured() bool {
	return c.FinanceFormula != 0 || c.SavingsPlanDiscount != 0 || c.OnDemandDiscount != 0
}
