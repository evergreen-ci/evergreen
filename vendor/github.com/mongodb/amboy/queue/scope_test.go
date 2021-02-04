package queue

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScopeManager(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, mngr ScopeManager){
		"AcquireChecksEachScopeForUniqueness": func(t *testing.T, mngr ScopeManager) {
			require.NoError(t, mngr.Acquire("id1", []string{"foo"}))
			require.NoError(t, mngr.Acquire("id1", []string{"bar", "bat"}))
			require.Error(t, mngr.Acquire("id2", []string{"foo"}))
			require.Error(t, mngr.Acquire("id2", []string{"foo", "baz"}))
		},
		"DoubleAcquireForSameIDWithSameScopesIsIdempotent": func(t *testing.T, mngr ScopeManager) {
			require.NoError(t, mngr.Acquire("id", []string{"foo"}))
			require.NoError(t, mngr.Acquire("id", []string{"foo"}))
		},
		"DoubleAcquireForSameIDWithDifferentScopesSucceeds": func(t *testing.T, mngr ScopeManager) {
			require.NoError(t, mngr.Acquire("id", []string{"foo"}))
			require.NoError(t, mngr.Acquire("id", []string{"bar"}))
		},
		"ReleaseOfUnownedScopeForIDIsNoop": func(t *testing.T, mngr ScopeManager) {
			require.NoError(t, mngr.Release("id", []string{"foo"}))
		},
		"ReleaseOfScopeOwnedByAnotherIDFails": func(t *testing.T, mngr ScopeManager) {
			require.NoError(t, mngr.Acquire("id1", []string{"foo"}))
			require.Error(t, mngr.Release("id2", []string{"foo"}))
		},
		"DoubleReleaseOfSameScopeForIDIsIdempotent": func(t *testing.T, mngr ScopeManager) {
			require.NoError(t, mngr.Acquire("id", []string{"foo"}))
			require.NoError(t, mngr.Release("id", []string{"foo"}))
			require.NoError(t, mngr.Release("id", []string{"foo"}))
		},
		"AcquireIsAtomic": func(t *testing.T, mngr ScopeManager) {
			require.NoError(t, mngr.Acquire("id1", []string{"foo"}))
			require.Error(t, mngr.Acquire("id2", []string{"bar", "foo"}))
			// If Acquire is atomic, "bar" should be unowned so this should not
			// error.
			require.NoError(t, mngr.Release("id1", []string{"bar"}))
		},
		"ReleaseIsAtomic": func(t *testing.T, mngr ScopeManager) {
			require.NoError(t, mngr.Acquire("id1", []string{"foo"}))
			require.NoError(t, mngr.Acquire("id2", []string{"bar", "bat"}))
			require.Error(t, mngr.Release("id2", []string{"bar", "foo"}))
			// If Release is atomic, "bar" should still be owned by "id2" so
			// this should error.
			require.Error(t, mngr.Release("id1", []string{"bar"}))
		},
	} {
		t.Run(testName, func(t *testing.T) {
			testCase(t, NewLocalScopeManager())
		})
	}
}
