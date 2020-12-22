package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckURL(t *testing.T) {
	url := "issuelink.com"
	err := CheckURL(url)
	assert.Contains(t, err.Error(), "error parsing request uri 'issuelink.com'")

	url = "http://issuelink.com/"
	err = CheckURL(url)
	assert.NoError(t, err)

	url = "http://issuelinkcom/ticket"
	err = CheckURL(url)
	assert.Contains(t, err.Error(), "url 'http://issuelinkcom/ticket' must have a domain and extension")

	url = "https://issuelink.com/browse/ticket"
	err = CheckURL(url)
	assert.NoError(t, err)

	url = "https://"
	err = CheckURL(url)
	assert.Contains(t, err.Error(), "url 'https://' must have a host name")

	url = "vscode://issuelink.com"
	err = CheckURL(url)
	assert.Contains(t, err.Error(), "url 'vscode://issuelink.com' scheme 'vscode' should either be 'http' or 'https'")
}
