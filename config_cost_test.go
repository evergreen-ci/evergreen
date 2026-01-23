package evergreen

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCostConfigValidateAndDefault(t *testing.T) {
	t.Run("ValidFinanceFields", func(t *testing.T) {
		c := CostConfig{
			FinanceFormula:      0.5,
			SavingsPlanDiscount: 0.3,
			OnDemandDiscount:    0.2,
		}
		assert.NoError(t, c.ValidateAndDefault())
	})

	t.Run("InvalidFinanceFormula", func(t *testing.T) {
		c := CostConfig{FinanceFormula: 1.5}
		assert.Error(t, c.ValidateAndDefault())

		c = CostConfig{FinanceFormula: -0.1}
		assert.Error(t, c.ValidateAndDefault())
	})

	t.Run("InvalidSavingsPlanDiscount", func(t *testing.T) {
		c := CostConfig{SavingsPlanDiscount: 1.5}
		assert.Error(t, c.ValidateAndDefault())

		c = CostConfig{SavingsPlanDiscount: -0.1}
		assert.Error(t, c.ValidateAndDefault())
	})

	t.Run("InvalidOnDemandDiscount", func(t *testing.T) {
		c := CostConfig{OnDemandDiscount: 1.5}
		assert.Error(t, c.ValidateAndDefault())

		c = CostConfig{OnDemandDiscount: -0.1}
		assert.Error(t, c.ValidateAndDefault())
	})

	t.Run("ValidS3UploadCostDiscount", func(t *testing.T) {
		c := CostConfig{
			S3Cost: S3CostConfig{
				Upload: S3UploadCostConfig{UploadCostDiscount: 0.0},
			},
		}
		assert.NoError(t, c.ValidateAndDefault())

		c.S3Cost.Upload.UploadCostDiscount = 0.5
		assert.NoError(t, c.ValidateAndDefault())

		c.S3Cost.Upload.UploadCostDiscount = 1.0
		assert.NoError(t, c.ValidateAndDefault())
	})

	t.Run("InvalidS3UploadCostDiscount", func(t *testing.T) {
		c := CostConfig{
			S3Cost: S3CostConfig{
				Upload: S3UploadCostConfig{UploadCostDiscount: -0.1},
			},
		}
		assert.Error(t, c.ValidateAndDefault())

		c.S3Cost.Upload.UploadCostDiscount = 1.5
		assert.Error(t, c.ValidateAndDefault())
	})

	t.Run("ValidS3StorageCostDiscounts", func(t *testing.T) {
		c := CostConfig{
			S3Cost: S3CostConfig{
				Storage: S3StorageCostConfig{
					StandardStorageCostDiscount:         0.0,
					InfrequentAccessStorageCostDiscount: 0.5,
				},
			},
		}
		assert.NoError(t, c.ValidateAndDefault())
	})

	t.Run("InvalidS3StandardStorageCostDiscount", func(t *testing.T) {
		c := CostConfig{
			S3Cost: S3CostConfig{
				Storage: S3StorageCostConfig{
					StandardStorageCostDiscount: -0.1,
				},
			},
		}
		assert.Error(t, c.ValidateAndDefault())

		c.S3Cost.Storage.StandardStorageCostDiscount = 1.5
		assert.Error(t, c.ValidateAndDefault())
	})

	t.Run("InvalidS3InfrequentAccessStorageCostDiscount", func(t *testing.T) {
		c := CostConfig{
			S3Cost: S3CostConfig{
				Storage: S3StorageCostConfig{
					InfrequentAccessStorageCostDiscount: -0.1,
				},
			},
		}
		assert.Error(t, c.ValidateAndDefault())

		c.S3Cost.Storage.InfrequentAccessStorageCostDiscount = 1.5
		assert.Error(t, c.ValidateAndDefault())
	})

	t.Run("MultipleInvalidFields", func(t *testing.T) {
		c := CostConfig{
			FinanceFormula:      1.5,
			SavingsPlanDiscount: -0.1,
			S3Cost: S3CostConfig{
				Upload: S3UploadCostConfig{UploadCostDiscount: -0.1},
				Storage: S3StorageCostConfig{
					StandardStorageCostDiscount: 1.5,
				},
			},
		}
		err := c.ValidateAndDefault()
		assert.Error(t, err)
	})
}

func TestCostConfigIsConfigured(t *testing.T) {
	t.Run("EmptyConfig", func(t *testing.T) {
		c := CostConfig{}
		assert.False(t, c.IsConfigured())
	})

	t.Run("FinanceFormulaSet", func(t *testing.T) {
		c := CostConfig{FinanceFormula: 0.5}
		assert.True(t, c.IsConfigured())
	})

	t.Run("SavingsPlanDiscountSet", func(t *testing.T) {
		c := CostConfig{SavingsPlanDiscount: 0.3}
		assert.True(t, c.IsConfigured())
	})

	t.Run("OnDemandDiscountSet", func(t *testing.T) {
		c := CostConfig{OnDemandDiscount: 0.2}
		assert.True(t, c.IsConfigured())
	})

	t.Run("S3UploadCostDiscountSetToNonZero", func(t *testing.T) {
		c := CostConfig{
			S3Cost: S3CostConfig{
				Upload: S3UploadCostConfig{UploadCostDiscount: 0.5},
			},
		}
		assert.True(t, c.IsConfigured())
	})

	t.Run("AllS3FieldsSet", func(t *testing.T) {
		c := CostConfig{
			S3Cost: S3CostConfig{
				Upload: S3UploadCostConfig{UploadCostDiscount: 0.1},
				Storage: S3StorageCostConfig{
					StandardStorageCostDiscount:         0.2,
					InfrequentAccessStorageCostDiscount: 0.3,
				},
			},
		}
		assert.True(t, c.IsConfigured())
	})

	t.Run("MixedFinanceAndS3Fields", func(t *testing.T) {
		c := CostConfig{
			FinanceFormula: 0.5,
			S3Cost: S3CostConfig{
				Upload: S3UploadCostConfig{UploadCostDiscount: 0.1},
			},
		}
		assert.True(t, c.IsConfigured())
	})
}

func TestCostConfigSetAndGet(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Run("SetAndGetFinanceFields", func(t *testing.T) {
		require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
		defer func() {
			require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
		}()

		original := CostConfig{
			FinanceFormula:      0.5,
			SavingsPlanDiscount: 0.3,
			OnDemandDiscount:    0.2,
		}
		require.NoError(t, original.Set(ctx))

		retrieved := CostConfig{}
		require.NoError(t, retrieved.Get(ctx))

		assert.Equal(t, original.FinanceFormula, retrieved.FinanceFormula)
		assert.Equal(t, original.SavingsPlanDiscount, retrieved.SavingsPlanDiscount)
		assert.Equal(t, original.OnDemandDiscount, retrieved.OnDemandDiscount)
	})

	t.Run("SetAndGetS3CostWithZeroValues", func(t *testing.T) {
		require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
		defer func() {
			require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
		}()

		original := CostConfig{
			S3Cost: S3CostConfig{
				Upload: S3UploadCostConfig{UploadCostDiscount: 0.0},
				Storage: S3StorageCostConfig{
					StandardStorageCostDiscount:         0.0,
					InfrequentAccessStorageCostDiscount: 0.0,
				},
			},
		}
		require.NoError(t, original.Set(ctx))

		retrieved := CostConfig{}
		require.NoError(t, retrieved.Get(ctx))

		assert.Equal(t, 0.0, retrieved.S3Cost.Upload.UploadCostDiscount)
		assert.Equal(t, 0.0, retrieved.S3Cost.Storage.StandardStorageCostDiscount)
		assert.Equal(t, 0.0, retrieved.S3Cost.Storage.InfrequentAccessStorageCostDiscount)
	})

	t.Run("SetAndGetS3CostWithNonZeroValues", func(t *testing.T) {
		require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
		defer func() {
			require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
		}()

		original := CostConfig{
			S3Cost: S3CostConfig{
				Upload: S3UploadCostConfig{UploadCostDiscount: 0.1},
				Storage: S3StorageCostConfig{
					StandardStorageCostDiscount:         0.2,
					InfrequentAccessStorageCostDiscount: 0.3,
				},
			},
		}
		require.NoError(t, original.Set(ctx))

		retrieved := CostConfig{}
		require.NoError(t, retrieved.Get(ctx))

		assert.Equal(t, 0.1, retrieved.S3Cost.Upload.UploadCostDiscount)
		assert.Equal(t, 0.2, retrieved.S3Cost.Storage.StandardStorageCostDiscount)
		assert.Equal(t, 0.3, retrieved.S3Cost.Storage.InfrequentAccessStorageCostDiscount)
	})

	t.Run("SetAndGetMixedFinanceAndS3Fields", func(t *testing.T) {
		require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
		defer func() {
			require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
		}()

		original := CostConfig{
			FinanceFormula:      0.5,
			SavingsPlanDiscount: 0.3,
			OnDemandDiscount:    0.2,
			S3Cost: S3CostConfig{
				Upload: S3UploadCostConfig{UploadCostDiscount: 0.1},
				Storage: S3StorageCostConfig{
					StandardStorageCostDiscount: 0.2,
				},
			},
		}
		require.NoError(t, original.Set(ctx))

		retrieved := CostConfig{}
		require.NoError(t, retrieved.Get(ctx))

		assert.Equal(t, original.FinanceFormula, retrieved.FinanceFormula)
		assert.Equal(t, original.SavingsPlanDiscount, retrieved.SavingsPlanDiscount)
		assert.Equal(t, original.OnDemandDiscount, retrieved.OnDemandDiscount)

		assert.Equal(t, 0.1, retrieved.S3Cost.Upload.UploadCostDiscount)
		assert.Equal(t, 0.2, retrieved.S3Cost.Storage.StandardStorageCostDiscount)
		assert.Equal(t, 0.0, retrieved.S3Cost.Storage.InfrequentAccessStorageCostDiscount)
	})
}
