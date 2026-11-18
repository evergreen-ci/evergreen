package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
)

func TestValidateBbProjectWebhook(t *testing.T) {
	validEndpoint := "https://build-baron.example.com/hooks"

	tests := map[string]struct {
		bbSettings   evergreen.BuildBaronSettings
		webhook      *evergreen.WebHook
		expectErrors []string
	}{
		"ValidEndpointWithSecretShouldPass": {
			webhook: &evergreen.WebHook{
				Endpoint: validEndpoint,
				Secret:   "shhh",
			},
		},
		"ValidEndpointWithoutSecretShouldPass": {
			webhook: &evergreen.WebHook{
				Endpoint: validEndpoint,
			},
		},
		"SecretWithoutEndpointShouldError": {
			webhook: &evergreen.WebHook{
				Secret: "shhh",
			},
			expectErrors: []string{"must provide webhook endpoint if webhook secret is provided"},
		},
		"EndpointWithInvalidSchemeShouldError": {
			webhook: &evergreen.WebHook{
				Endpoint: "ftp://build-baron.example.com/hooks",
			},
			expectErrors: []string{"validating webhook endpoint"},
		},
		"RelativeEndpointShouldError": {
			webhook: &evergreen.WebHook{
				Endpoint: "/api/route",
			},
			expectErrors: []string{"validating webhook endpoint"},
		},
		"LoopbackEndpointShouldError": {
			webhook: &evergreen.WebHook{
				Endpoint: "http://127.0.0.1:8080/hooks",
			},
			expectErrors: []string{"validating webhook endpoint"},
		},
		"WebhookAndBuildBaronConfiguredShouldError": {
			bbSettings: evergreen.BuildBaronSettings{
				TicketCreateProject: "BFG",
			},
			webhook: &evergreen.WebHook{
				Endpoint: validEndpoint,
			},
			expectErrors: []string{"The custom file ticket webhook and the build baron should not both be configured"},
		},
		"EmptyConfigurationShouldPass": {
			bbSettings: evergreen.BuildBaronSettings{},
			webhook:    &evergreen.WebHook{},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := ValidateBbProject(t.Context(), "proj", test.bbSettings, test.webhook)
			if len(test.expectErrors) == 0 {
				assert.NoError(t, err)
			} else {
				for _, expectedErr := range test.expectErrors {
					assert.ErrorContains(t, err, expectedErr)
				}
			}
		})
	}
}
