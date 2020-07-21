package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCleanName(t *testing.T) {
	assert.Equal(t, "Int", cleanName("int", false))
	assert.Equal(t, "IntPtr", cleanName("int", true))
	assert.Equal(t, "Arrint", cleanName("[]int", false))
	assert.Equal(t, "Mapstringstring", cleanName("map[string]string", false))
	assert.Equal(t, "Interface", cleanName("interface{}", false))
	assert.Equal(t, "Mapstringinterface", cleanName("map[string]interface{}", false))
}
