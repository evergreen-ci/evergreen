package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckURL(t *testing.T) {
	url := "issuelink.com"
	err := CheckURL(url)
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "parsing request URI 'issuelink.com'")

	url = "http://issuelink.com/"
	err = CheckURL(url)
	assert.NoError(t, err)

	url = "http://issuelinkcom/ticket"
	err = CheckURL(url)
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "URL 'http://issuelinkcom/ticket' must have a domain and extension")

	url = "https://issuelink.com/browse/ticket"
	err = CheckURL(url)
	assert.NoError(t, err)

	url = "https://"
	err = CheckURL(url)
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "URL 'https://' must have a host name")

	url = "vscode://issuelink.com"
	err = CheckURL(url)
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "scheme 'vscode' for URL 'vscode://issuelink.com' should either be 'http' or 'https'")
}
