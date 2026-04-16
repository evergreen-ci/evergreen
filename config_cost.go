package evergreen

import (
	"context"
	"slices"
	"strings"

	"github.com/evergreen-ci/evergreen/util"
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
	// S3Cost holds S3-related cost discount configuration
	S3Cost S3CostConfig `bson:"s3_cost" json:"s3_cost" yaml:"s3_cost"`
	// EBSCost holds EBS-related cost discount configuration
	EBSCost EBSCostConfig `bson:"ebs_cost" json:"ebs_cost" yaml:"ebs_cost"`
}

// S3UploadCostConfig represents S3 upload cost discount configuration.
type S3UploadCostConfig struct {
	UploadCostDiscount float64 `bson:"upload_cost_discount" json:"upload_cost_discount" yaml:"upload_cost_discount"`
}

// S3StorageCostConfig represents S3 storage cost discount configuration.
type S3StorageCostConfig struct {
	StandardStorageCostDiscount float64 `bson:"standard_storage_cost_discount" json:"standard_storage_cost_discount" yaml:"standard_storage_cost_discount"`
	IAStorageCostDiscount       float64 `bson:"i_a_storage_cost_discount" json:"i_a_storage_cost_discount" yaml:"i_a_storage_cost_discount"`
	ArchiveStorageCostDiscount  float64 `bson:"archive_storage_cost_discount" json:"archive_storage_cost_discount" yaml:"archive_storage_cost_discount"`
	// DefaultMaxArtifactExpirationDays is the fallback retention period used when no lifecycle rule is found for a bucket.
	DefaultMaxArtifactExpirationDays int `bson:"default_max_artifact_expiration_days" json:"default_max_artifact_expiration_days" yaml:"default_max_artifact_expiration_days"`
	// DevprodOwnedAWSAccountIDs contains the AWS account IDs of the accounts that are owned by Devprod that we want to calculate s3 costs for.
	DevprodOwnedAWSAccountIDs []string `bson:"devprod_owned_aws_account_ids,omitempty" json:"devprod_owned_aws_account_ids,omitempty" yaml:"devprod_owned_aws_account_ids,omitempty"`
	// ArtifactAWSAccountsWithoutLifecycleRules contains the AWS account IDs of the accounts that we want to calculate s3 costs for but don't have lifecycle rules for.
	ArtifactAWSAccountsWithoutLifecycleRules []string `bson:"artifact_aws_accounts_without_lifecycle_rules,omitempty" json:"artifact_aws_accounts_without_lifecycle_rules,omitempty" yaml:"artifact_aws_accounts_without_lifecycle_rules,omitempty"`
}

// S3CostConfig represents S3 cost configuration with separate upload and storage settings.
type S3CostConfig struct {
	Upload  S3UploadCostConfig  `bson:"upload" json:"upload" yaml:"upload"`
	Storage S3StorageCostConfig `bson:"storage" json:"storage" yaml:"storage"`
}

// EBSCostConfig holds EBS-related cost discount configuration.
type EBSCostConfig struct {
	// EBSDiscount is the discount rate (0.0-1.0) applied to EBS costs (throughput, storage, etc.).
	EBSDiscount float64 `bson:"ebs_discount" json:"ebs_discount" yaml:"ebs_discount"`
}

var (
	financeConfigFormulaKey             = bsonutil.MustHaveTag(CostConfig{}, "FinanceFormula")
	financeConfigSavingsPlanDiscountKey = bsonutil.MustHaveTag(CostConfig{}, "SavingsPlanDiscount")
	financeConfigOnDemandDiscountKey    = bsonutil.MustHaveTag(CostConfig{}, "OnDemandDiscount")
	financeConfigS3CostKey              = bsonutil.MustHaveTag(CostConfig{}, "S3Cost")
	financeConfigEBSCostKey             = bsonutil.MustHaveTag(CostConfig{}, "EBSCost")

	s3CostConfigUploadKey  = bsonutil.MustHaveTag(S3CostConfig{}, "Upload")
	s3CostConfigStorageKey = bsonutil.MustHaveTag(S3CostConfig{}, "Storage")

	s3UploadCostUploadCostDiscountKey = bsonutil.MustHaveTag(S3UploadCostConfig{}, "UploadCostDiscount")

	s3StorageCostStandardStorageCostDiscountKey              = bsonutil.MustHaveTag(S3StorageCostConfig{}, "StandardStorageCostDiscount")
	s3StorageCostIAStorageCostDiscountKey                    = bsonutil.MustHaveTag(S3StorageCostConfig{}, "IAStorageCostDiscount")
	s3StorageCostArchiveStorageCostDiscountKey               = bsonutil.MustHaveTag(S3StorageCostConfig{}, "ArchiveStorageCostDiscount")
	s3StorageCostDefaultMaxArtifactExpirationDaysKey         = bsonutil.MustHaveTag(S3StorageCostConfig{}, "DefaultMaxArtifactExpirationDays")
	s3StorageCostDevprodOwnedAWSAccountIDsKey                = bsonutil.MustHaveTag(S3StorageCostConfig{}, "DevprodOwnedAWSAccountIDs")
	s3StorageCostArtifactAWSAccountsWithoutLifecycleRulesKey = bsonutil.MustHaveTag(S3StorageCostConfig{}, "ArtifactAWSAccountsWithoutLifecycleRules")
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
			bsonutil.GetDottedKeyName(financeConfigS3CostKey, s3CostConfigUploadKey, s3UploadCostUploadCostDiscountKey):                         c.S3Cost.Upload.UploadCostDiscount,
			bsonutil.GetDottedKeyName(financeConfigS3CostKey, s3CostConfigStorageKey, s3StorageCostStandardStorageCostDiscountKey):              c.S3Cost.Storage.StandardStorageCostDiscount,
			bsonutil.GetDottedKeyName(financeConfigS3CostKey, s3CostConfigStorageKey, s3StorageCostIAStorageCostDiscountKey):                    c.S3Cost.Storage.IAStorageCostDiscount,
			bsonutil.GetDottedKeyName(financeConfigS3CostKey, s3CostConfigStorageKey, s3StorageCostArchiveStorageCostDiscountKey):               c.S3Cost.Storage.ArchiveStorageCostDiscount,
			bsonutil.GetDottedKeyName(financeConfigS3CostKey, s3CostConfigStorageKey, s3StorageCostDefaultMaxArtifactExpirationDaysKey):         c.S3Cost.Storage.DefaultMaxArtifactExpirationDays,
			bsonutil.GetDottedKeyName(financeConfigS3CostKey, s3CostConfigStorageKey, s3StorageCostDevprodOwnedAWSAccountIDsKey):                c.S3Cost.Storage.DevprodOwnedAWSAccountIDs,
			bsonutil.GetDottedKeyName(financeConfigS3CostKey, s3CostConfigStorageKey, s3StorageCostArtifactAWSAccountsWithoutLifecycleRulesKey): c.S3Cost.Storage.ArtifactAWSAccountsWithoutLifecycleRules,
			financeConfigEBSCostKey: c.EBSCost,
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
	validateDiscountField(c.S3Cost.Storage.ArchiveStorageCostDiscount, "S3 archive storage cost discount", catcher)
	validateDiscountField(c.EBSCost.EBSDiscount, "EBS cost discount", catcher)
	if c.S3Cost.Storage.DefaultMaxArtifactExpirationDays < 0 {
		catcher.New("default max artifact expiration days must be non-negative")
	}
	for _, id := range c.S3Cost.Storage.DevprodOwnedAWSAccountIDs {
		catcher.Add(validateAWSAccountID(id, "devprod owned AWS account ID"))
	}
	for _, id := range c.S3Cost.Storage.ArtifactAWSAccountsWithoutLifecycleRules {
		catcher.Add(validateAWSAccountID(id, "artifact AWS account without lifecycle rules"))
	}

	return catcher.Resolve()
}

func validateAWSAccountID(id, fieldLabel string) error {
	s := strings.TrimSpace(id)
	if s == "" {
		return errors.Errorf("%s cannot be empty", fieldLabel)
	}
	if len(s) != 12 {
		return errors.Errorf("%s must be a 12-digit AWS account ID", fieldLabel)
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return errors.Errorf("%s must contain only digits", fieldLabel)
		}
	}
	return nil
}

// IsInDevProdOwnedAccountList reports whether an account ID is in the list of devprod owned account IDs.
func IsInDevProdOwnedAccountList(accountID string, list []string) bool {
	return slices.ContainsFunc(list, func(id string) bool {
		return strings.TrimSpace(id) == accountID
	})
}

// IsDevprodOwnedArtifactIAMRole reports whether an IAM role ARN belongs to an account in the list
// of devprod owned account IDs.
func IsDevprodOwnedArtifactIAMRole(awsRoleARN string, devprodOwnedAWSAccountIDs []string) bool {
	if len(devprodOwnedAWSAccountIDs) == 0 {
		return true
	}
	acctID, ok := util.AWSAccountIDFromIAMARN(awsRoleARN)
	return ok && IsInDevProdOwnedAccountList(acctID, devprodOwnedAWSAccountIDs)
}

// IsConfigured returns true if any finance config field is set.
func (c *CostConfig) IsConfigured() bool {
	return c.FinanceFormula != 0 ||
		c.SavingsPlanDiscount != 0 ||
		c.OnDemandDiscount != 0 ||
		c.S3Cost.Upload.UploadCostDiscount != 0 ||
		c.S3Cost.Storage.StandardStorageCostDiscount != 0 ||
		c.S3Cost.Storage.IAStorageCostDiscount != 0 ||
		c.S3Cost.Storage.ArchiveStorageCostDiscount != 0 ||
		c.EBSCost.EBSDiscount != 0 ||
		c.S3Cost.Storage.DefaultMaxArtifactExpirationDays != 0
}
