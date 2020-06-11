package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCleanName(t *testing.T) {
	assert.Equal(t, "Int", cleanName("int"))
	assert.Equal(t, "Arrint", cleanName("[]int"))
	assert.Equal(t, "Mapstringstring", cleanName("map[string]string"))
	assert.Equal(t, "Interface", cleanName("interface{}"))
	assert.Equal(t, "Mapstringinterface", cleanName("map[string]interface{}"))
}
