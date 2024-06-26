package thirdparty

// TODO: Uncomment when DEVPROD-6983 is resolved. Right now, the API does not work on task hosts.
// func TestGetImageNames(t *testing.T) {
// 	assert := assert.New(t)
// 	config := testutil.TestConfig()
// 	testutil.ConfigureIntegrationTest(t, config, "TestGetImageNames")
// 	result, err := getImageNames(context.TODO(), config.RuntimeEnvironments.BaseURL, config.RuntimeEnvironments.APIKey)
// 	assert.NoError(err)
// 	assert.NotEmpty(result)
// 	assert.NotContains(result, "")
// }
