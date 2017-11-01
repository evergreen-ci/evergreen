package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConstructor(t *testing.T) {
	assert := assert.New(t)

	info := &requestInfo{}
	comm := &communicatorImpl{}

	_, err := newPaginatorHelper(nil, comm)
	assert.Error(err)
	_, err = newPaginatorHelper(info, nil)
	assert.Error(err)

	pgHelper, err := newPaginatorHelper(info, comm)
	assert.NoError(err)
	assert.NotNil(pgHelper.routeInfo)
	assert.NotNil(pgHelper.comm)
	assert.True(pgHelper.hasMore())
	pgHelper.setNextPagePath("test")
	assert.Equal("test", pgHelper.getNextPagePath())
}

func TestLinkParsing(t *testing.T) {
	assert := assert.New(t) // nolint

	test1 := `<http://localhost:8080/api/rest/v2/users/admin/hosts?host_id=foo>; rel="next"`
	expected1 := `/users/admin/hosts?host_id=foo`
	test2 := ""
	expected2 := ""
	test3 := "<invalid>"
	expected3 := ""
	version := apiVersion2

	assert.Equal(expected1, parseLink(test1, string(version)))
	assert.Equal(expected2, parseLink(test2, string(version)))
	assert.Equal(expected3, parseLink(test3, string(version)))
}
