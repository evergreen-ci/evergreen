package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
	assert := assert.New(t)

	myDinner := dinner{
		Entree:  "gefilte fish",
		Dessert: true,
		Drink: &drink{
			Sugars: 32,
		},
	}
	var yourDinner dinner
	err := DeepCopy(myDinner, &yourDinner, nil)
	assert.NoError(err)
	assert.Equal(myDinner.Entree, yourDinner.Entree)
	assert.Equal(myDinner.Drink.Sugars, yourDinner.Drink.Sugars)
	assert.Equal(myDinner.Dessert, yourDinner.Dessert)
}
