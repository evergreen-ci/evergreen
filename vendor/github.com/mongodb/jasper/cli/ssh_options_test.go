package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClientOptions(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, opts *ClientOptions){
		"ValidateSucceedsWithValidOptions": func(t *testing.T, opts *ClientOptions) {
			assert.NoError(t, opts.Validate())
		},
		"ValidateFailsWithEmptyOptions": func(t *testing.T, opts *ClientOptions) {
			opts = &ClientOptions{}
			assert.Error(t, opts.Validate())
		},
		"ValidateFailsWithoutBinaryPath": func(t *testing.T, opts *ClientOptions) {
			opts.BinaryPath = ""
			assert.Error(t, opts.Validate())
		},
		"ValidateFailsWithoutType": func(t *testing.T, opts *ClientOptions) {
			opts.Type = ""
			assert.Error(t, opts.Validate())
		},
		"ValidateFailsWithInvalidType": func(t *testing.T, opts *ClientOptions) {
			opts.Type = "invalid"
			assert.Error(t, opts.Validate())
		},
	} {
		t.Run(testName, func(t *testing.T) {
			opts := ClientOptions{
				BinaryPath:          "binary",
				Type:                RPCService,
				Host:                "localhost",
				Port:                12345,
				CredentialsFilePath: "/path/to/credentials",
			}
			testCase(t, &opts)
		})
	}
}
