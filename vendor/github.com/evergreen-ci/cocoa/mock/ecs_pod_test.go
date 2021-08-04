package mock

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/ecs"
	"github.com/evergreen-ci/cocoa/internal/testcase"
	"github.com/evergreen-ci/cocoa/internal/testutil"
	"github.com/evergreen-ci/cocoa/secret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestECSPod(t *testing.T) {
	assert.Implements(t, (*cocoa.ECSPod)(nil), &ECSPod{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	GlobalECSService.Clusters[testutil.ECSClusterName()] = ECSCluster{}

	c := &ECSClient{}
	defer func() {
		assert.NoError(t, c.Close(ctx))
	}()

	for tName, tCase := range testcase.ECSPodTests() {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, time.Second)
			defer tcancel()

			sm := &SecretsManagerClient{}
			defer func() {
				assert.NoError(t, c.Close(tctx))
			}()
			v := NewVault(secret.NewBasicSecretsManager(sm))

			pc, err := ecs.NewBasicECSPodCreator(c, v)
			require.NoError(t, err)
			podCreator := NewECSPodCreator(pc)

			tCase(tctx, t, v, podCreator)
		})
	}
}
