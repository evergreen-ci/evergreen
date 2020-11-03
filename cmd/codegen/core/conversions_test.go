package core

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

func TestShortenPackage(t *testing.T) {
	assert.Equal(t, "[]b.Struct", shortenPackage("[]package/a/b.Struct"))
	assert.Equal(t, "[]b.Struct", shortenPackage("[]b.Struct"))
	assert.Equal(t, "", shortenPackage(""))
	assert.Equal(t, "*foo", shortenPackage("*foo"))
	assert.Equal(t, "package.foo", shortenPackage("package.foo"))
	assert.Equal(t, "package.foo", shortenPackage("something/else/package.foo"))
}
