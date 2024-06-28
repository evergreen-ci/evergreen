package thirdparty

import (
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestGetImageNames(t *testing.T) {
	assert := assert.New(t)
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config, "TestGetImageNames")
	c := RuntimeEnvironmentsClient{
		Client:  &http.Client{},
		BaseURL: config.RuntimeEnvironments.BaseURL,
		APIKey:  config.RuntimeEnvironments.APIKey,
	}
	result, err := c.getImageNames(context.TODO())
	assert.NoError(err)
	assert.NotEmpty(result)
	assert.NotContains(result, "")
}
