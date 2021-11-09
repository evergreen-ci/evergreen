package mock

import (
	"context"
	"testing"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/internal/testcase"
	"github.com/stretchr/testify/assert"
)

func TestSecretsManagerClient(t *testing.T) {
	assert.Implements(t, (*cocoa.SecretsManagerClient)(nil), &SecretsManagerClient{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for tName, tCase := range testcase.SecretsManagerClientTests() {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, defaultTestTimeout)
			defer tcancel()

			cleanupECSAndSecretsManagerCache()

			c := &SecretsManagerClient{}
			defer func() {
				assert.NoError(t, c.Close(tctx))
			}()

			tCase(tctx, t, c)
		})
	}
}
