package util

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type dinner struct {
	Entree  string
	Drink   *drink
	Dessert bool
}

type drink struct {
	Sugars int
}

func TestDeepCopy(t *testing.T) {
	require := require.New(t)
	t.Run("DeepCopy", func(t *testing.T) {
		myDinner := dinner{
			Entree:  "gefilte fish",
			Dessert: true,
			Drink: &drink{
				Sugars: 32,
			},
		}
		var yourDinner dinner
		err := DeepCopy(myDinner, &yourDinner)
		require.NoError(err)
		require.Equal(myDinner.Entree, yourDinner.Entree)
		require.Equal(myDinner.Drink.Sugars, yourDinner.Drink.Sugars)
		require.Equal(myDinner.Dessert, yourDinner.Dessert)
	})
	t.Run("DeepCopyWithAStringSlice", func(t *testing.T) {
		mySlice := []string{"foo", "bar"}
		var yourSlice []string
		err := DeepCopy(mySlice, &yourSlice)
		require.NoError(err)
		require.Equal(mySlice, yourSlice)
	})

	t.Run("DeepCopyWithAStringMap", func(t *testing.T) {
		myMap := map[string]string{
			"foo": "bar",
		}
		var yourMap map[string]string
		err := DeepCopy(myMap, &yourMap)
		require.NoError(err)
		require.Equal(myMap, yourMap)
	})
}
