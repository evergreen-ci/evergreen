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

	t.Run("S3CostNilFields", func(t *testing.T) {
		c := CostConfig{
			S3Cost: S3CostConfig{
				Upload: S3UploadCostConfig{UploadCostDiscount: nil},
				Storage: S3StorageCostConfig{
					StandardStorageCostDiscount:         nil,
					InfrequentAccessStorageCostDiscount: nil,
				},
			},
		}
		assert.NoError(t, c.ValidateAndDefault())
	})

	t.Run("ValidS3UploadCostDiscount", func(t *testing.T) {
		zero := 0.0
		half := 0.5
		one := 1.0

		c := CostConfig{
			S3Cost: S3CostConfig{
				Upload: S3UploadCostConfig{UploadCostDiscount: &zero},
			},
		}
		assert.NoError(t, c.ValidateAndDefault())

		c.S3Cost.Upload.UploadCostDiscount = &half
		assert.NoError(t, c.ValidateAndDefault())

		c.S3Cost.Upload.UploadCostDiscount = &one
		assert.NoError(t, c.ValidateAndDefault())
	})

	t.Run("InvalidS3UploadCostDiscount", func(t *testing.T) {
		negative := -0.1
		overOne := 1.5

		c := CostConfig{
			S3Cost: S3CostConfig{
				Upload: S3UploadCostConfig{UploadCostDiscount: &negative},
			},
		}
		assert.Error(t, c.ValidateAndDefault())

		c.S3Cost.Upload.UploadCostDiscount = &overOne
		assert.Error(t, c.ValidateAndDefault())
	})

	t.Run("ValidS3StorageCostDiscounts", func(t *testing.T) {
		zero := 0.0
		half := 0.5

		c := CostConfig{
			S3Cost: S3CostConfig{
				Storage: S3StorageCostConfig{
					StandardStorageCostDiscount:         &zero,
					InfrequentAccessStorageCostDiscount: &half,
				},
			},
		}
		assert.NoError(t, c.ValidateAndDefault())
	})

	t.Run("InvalidS3StandardStorageCostDiscount", func(t *testing.T) {
		negative := -0.1
		overOne := 1.5

		c := CostConfig{
			S3Cost: S3CostConfig{
				Storage: S3StorageCostConfig{
					StandardStorageCostDiscount: &negative,
				},
			},
		}
		assert.Error(t, c.ValidateAndDefault())

		c.S3Cost.Storage.StandardStorageCostDiscount = &overOne
		assert.Error(t, c.ValidateAndDefault())
	})

	t.Run("InvalidS3InfrequentAccessStorageCostDiscount", func(t *testing.T) {
		negative := -0.1
		overOne := 1.5

		c := CostConfig{
			S3Cost: S3CostConfig{
				Storage: S3StorageCostConfig{
					InfrequentAccessStorageCostDiscount: &negative,
				},
			},
		}
		assert.Error(t, c.ValidateAndDefault())

		c.S3Cost.Storage.InfrequentAccessStorageCostDiscount = &overOne
		assert.Error(t, c.ValidateAndDefault())
	})

	t.Run("MultipleInvalidFields", func(t *testing.T) {
		negative := -0.1
		overOne := 1.5

		c := CostConfig{
			FinanceFormula:      1.5,
			SavingsPlanDiscount: -0.1,
			S3Cost: S3CostConfig{
				Upload: S3UploadCostConfig{UploadCostDiscount: &negative},
				Storage: S3StorageCostConfig{
					StandardStorageCostDiscount: &overOne,
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

	t.Run("S3UploadCostDiscountNil", func(t *testing.T) {
		c := CostConfig{
			S3Cost: S3CostConfig{
				Upload: S3UploadCostConfig{UploadCostDiscount: nil},
			},
		}
		assert.False(t, c.IsConfigured())
	})

	t.Run("S3UploadCostDiscountSetToZero", func(t *testing.T) {
		zero := 0.0
		c := CostConfig{
			S3Cost: S3CostConfig{
				Upload: S3UploadCostConfig{UploadCostDiscount: &zero},
			},
		}
		assert.True(t, c.IsConfigured())
	})

	t.Run("S3UploadCostDiscountSetToNonZero", func(t *testing.T) {
		value := 0.5
		c := CostConfig{
			S3Cost: S3CostConfig{
				Upload: S3UploadCostConfig{UploadCostDiscount: &value},
			},
		}
		assert.True(t, c.IsConfigured())
	})

	t.Run("S3StandardStorageCostDiscountNil", func(t *testing.T) {
		c := CostConfig{
			S3Cost: S3CostConfig{
				Storage: S3StorageCostConfig{
					StandardStorageCostDiscount: nil,
				},
			},
		}
		assert.False(t, c.IsConfigured())
	})

	t.Run("S3StandardStorageCostDiscountSetToZero", func(t *testing.T) {
		zero := 0.0
		c := CostConfig{
			S3Cost: S3CostConfig{
				Storage: S3StorageCostConfig{
					StandardStorageCostDiscount: &zero,
				},
			},
		}
		assert.True(t, c.IsConfigured())
	})

	t.Run("S3InfrequentAccessStorageCostDiscountSetToZero", func(t *testing.T) {
		zero := 0.0
		c := CostConfig{
			S3Cost: S3CostConfig{
				Storage: S3StorageCostConfig{
					InfrequentAccessStorageCostDiscount: &zero,
				},
			},
		}
		assert.True(t, c.IsConfigured())
	})

	t.Run("AllS3FieldsSet", func(t *testing.T) {
		upload := 0.1
		standard := 0.2
		infrequent := 0.3

		c := CostConfig{
			S3Cost: S3CostConfig{
				Upload: S3UploadCostConfig{UploadCostDiscount: &upload},
				Storage: S3StorageCostConfig{
					StandardStorageCostDiscount:         &standard,
					InfrequentAccessStorageCostDiscount: &infrequent,
				},
			},
		}
		assert.True(t, c.IsConfigured())
	})

	t.Run("MixedFinanceAndS3Fields", func(t *testing.T) {
		upload := 0.1

		c := CostConfig{
			FinanceFormula: 0.5,
			S3Cost: S3CostConfig{
				Upload: S3UploadCostConfig{UploadCostDiscount: &upload},
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

	t.Run("SetAndGetS3CostWithNilFields", func(t *testing.T) {
		require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
		defer func() {
			require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
		}()

		original := CostConfig{
			S3Cost: S3CostConfig{
				Upload: S3UploadCostConfig{UploadCostDiscount: nil},
				Storage: S3StorageCostConfig{
					StandardStorageCostDiscount:         nil,
					InfrequentAccessStorageCostDiscount: nil,
				},
			},
		}
		require.NoError(t, original.Set(ctx))

		retrieved := CostConfig{}
		require.NoError(t, retrieved.Get(ctx))

		assert.Nil(t, retrieved.S3Cost.Upload.UploadCostDiscount)
		assert.Nil(t, retrieved.S3Cost.Storage.StandardStorageCostDiscount)
		assert.Nil(t, retrieved.S3Cost.Storage.InfrequentAccessStorageCostDiscount)
	})

	t.Run("SetAndGetS3CostWithZeroValues", func(t *testing.T) {
		require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
		defer func() {
			require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
		}()

		zero := 0.0
		original := CostConfig{
			S3Cost: S3CostConfig{
				Upload: S3UploadCostConfig{UploadCostDiscount: &zero},
				Storage: S3StorageCostConfig{
					StandardStorageCostDiscount:         &zero,
					InfrequentAccessStorageCostDiscount: &zero,
				},
			},
		}
		require.NoError(t, original.Set(ctx))

		retrieved := CostConfig{}
		require.NoError(t, retrieved.Get(ctx))

		require.NotNil(t, retrieved.S3Cost.Upload.UploadCostDiscount)
		assert.Equal(t, 0.0, *retrieved.S3Cost.Upload.UploadCostDiscount)

		require.NotNil(t, retrieved.S3Cost.Storage.StandardStorageCostDiscount)
		assert.Equal(t, 0.0, *retrieved.S3Cost.Storage.StandardStorageCostDiscount)

		require.NotNil(t, retrieved.S3Cost.Storage.InfrequentAccessStorageCostDiscount)
		assert.Equal(t, 0.0, *retrieved.S3Cost.Storage.InfrequentAccessStorageCostDiscount)
	})

	t.Run("SetAndGetS3CostWithNonZeroValues", func(t *testing.T) {
		require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
		defer func() {
			require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
		}()

		upload := 0.1
		standard := 0.2
		infrequent := 0.3

		original := CostConfig{
			S3Cost: S3CostConfig{
				Upload: S3UploadCostConfig{UploadCostDiscount: &upload},
				Storage: S3StorageCostConfig{
					StandardStorageCostDiscount:         &standard,
					InfrequentAccessStorageCostDiscount: &infrequent,
				},
			},
		}
		require.NoError(t, original.Set(ctx))

		retrieved := CostConfig{}
		require.NoError(t, retrieved.Get(ctx))

		require.NotNil(t, retrieved.S3Cost.Upload.UploadCostDiscount)
		assert.Equal(t, 0.1, *retrieved.S3Cost.Upload.UploadCostDiscount)

		require.NotNil(t, retrieved.S3Cost.Storage.StandardStorageCostDiscount)
		assert.Equal(t, 0.2, *retrieved.S3Cost.Storage.StandardStorageCostDiscount)

		require.NotNil(t, retrieved.S3Cost.Storage.InfrequentAccessStorageCostDiscount)
		assert.Equal(t, 0.3, *retrieved.S3Cost.Storage.InfrequentAccessStorageCostDiscount)
	})

	t.Run("SetAndGetMixedFinanceAndS3Fields", func(t *testing.T) {
		require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
		defer func() {
			require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
		}()

		upload := 0.1
		standard := 0.2

		original := CostConfig{
			FinanceFormula:      0.5,
			SavingsPlanDiscount: 0.3,
			OnDemandDiscount:    0.2,
			S3Cost: S3CostConfig{
				Upload: S3UploadCostConfig{UploadCostDiscount: &upload},
				Storage: S3StorageCostConfig{
					StandardStorageCostDiscount: &standard,
				},
			},
		}
		require.NoError(t, original.Set(ctx))

		retrieved := CostConfig{}
		require.NoError(t, retrieved.Get(ctx))

		assert.Equal(t, original.FinanceFormula, retrieved.FinanceFormula)
		assert.Equal(t, original.SavingsPlanDiscount, retrieved.SavingsPlanDiscount)
		assert.Equal(t, original.OnDemandDiscount, retrieved.OnDemandDiscount)

		require.NotNil(t, retrieved.S3Cost.Upload.UploadCostDiscount)
		assert.Equal(t, 0.1, *retrieved.S3Cost.Upload.UploadCostDiscount)

		require.NotNil(t, retrieved.S3Cost.Storage.StandardStorageCostDiscount)
		assert.Equal(t, 0.2, *retrieved.S3Cost.Storage.StandardStorageCostDiscount)

		assert.Nil(t, retrieved.S3Cost.Storage.InfrequentAccessStorageCostDiscount)
	})
}
