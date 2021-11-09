package queue

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScopeManager(t *testing.T) {
	for acquireType, acquire := range map[string]func(mngr ScopeManager, id string, scopes []string) error{
		"Acquire": func(mngr ScopeManager, id string, scopes []string) error {
			return mngr.Acquire(id, scopes)
		},
		"ReleaseAndAcquire": func(mngr ScopeManager, id string, scopes []string) error {
			return mngr.ReleaseAndAcquire("", nil, id, scopes)
		},
	} {
		t.Run(fmt.Sprintf("AcquireWith%sMethod", acquireType), func(t *testing.T) {
			for releaseType, release := range map[string]func(mngr ScopeManager, id string, scopes []string) error{
				"Release": func(mngr ScopeManager, id string, scopes []string) error {
					return mngr.Release(id, scopes)
				},
				"ReleaseAndAcquire": func(mngr ScopeManager, id string, scopes []string) error {
					return mngr.ReleaseAndAcquire(id, scopes, "", nil)
				},
			} {
				t.Run(fmt.Sprintf("ReleaseWith%sMethod", releaseType), func(t *testing.T) {
					for testName, testCase := range map[string]func(t *testing.T, mngr ScopeManager){
						"AcquireChecksEachScopeForUniqueness": func(t *testing.T, mngr ScopeManager) {
							require.NoError(t, acquire(mngr, "id1", []string{"foo"}))
							require.NoError(t, acquire(mngr, "id1", []string{"bar", "bat"}))
							require.Error(t, acquire(mngr, "id2", []string{"foo"}))
							require.Error(t, acquire(mngr, "id2", []string{"foo", "baz"}))
						},
						"DoubleAcquireForSameIDWithSameScopesIsIdempotent": func(t *testing.T, mngr ScopeManager) {
							require.NoError(t, acquire(mngr, "id", []string{"foo"}))
							require.NoError(t, acquire(mngr, "id", []string{"foo"}))
						},
						"DoubleAcquireForSameIDWithDifferentScopesSucceeds": func(t *testing.T, mngr ScopeManager) {
							require.NoError(t, acquire(mngr, "id", []string{"foo"}))
							require.NoError(t, acquire(mngr, "id", []string{"bar"}))
						},
						"AcquireIsAtomic": func(t *testing.T, mngr ScopeManager) {
							require.NoError(t, acquire(mngr, "id1", []string{"foo"}))
							require.Error(t, acquire(mngr, "id2", []string{"bar", "foo"}))
							// If Acquire is atomic, "bar" should be unowned so this should not
							// error.
							require.NoError(t, release(mngr, "id1", []string{"bar"}))
						},
						"ReleaseOfUnownedScopeForIDIsNoop": func(t *testing.T, mngr ScopeManager) {
							require.NoError(t, release(mngr, "id", []string{"foo"}))
						},
						"ReleaseOfScopeOwnedByAnotherIDFails": func(t *testing.T, mngr ScopeManager) {
							require.NoError(t, acquire(mngr, "id1", []string{"foo"}))
							require.Error(t, release(mngr, "id2", []string{"foo"}))
						},
						"DoubleReleaseOfSameScopeForIDIsIdempotent": func(t *testing.T, mngr ScopeManager) {
							require.NoError(t, acquire(mngr, "id", []string{"foo"}))
							require.NoError(t, release(mngr, "id", []string{"foo"}))
							require.NoError(t, release(mngr, "id", []string{"foo"}))
						},
						"ReleaseIsAtomic": func(t *testing.T, mngr ScopeManager) {
							require.NoError(t, acquire(mngr, "id1", []string{"foo"}))
							require.NoError(t, acquire(mngr, "id2", []string{"bar", "bat"}))
							require.Error(t, release(mngr, "id2", []string{"bar", "foo"}))
							// If Release is atomic, "bar" should still be owned by "id2" so
							// this should error.
							require.Error(t, release(mngr, "id1", []string{"bar"}))
						},
					} {
						t.Run(testName, func(t *testing.T) {
							testCase(t, NewLocalScopeManager())
						})
					}
				})
			}
		})
	}
	t.Run("ReleaseAndAcquire", func(t *testing.T) {
		for testName, testCase := range map[string]func(t *testing.T, mngr ScopeManager){
			"AllowsJustRelease": func(t *testing.T, mngr ScopeManager) {
				require.NoError(t, mngr.ReleaseAndAcquire("id1", []string{"foo"}, "", nil))
			},
			"AllowsJustAcquire": func(t *testing.T, mngr ScopeManager) {
				require.NoError(t, mngr.ReleaseAndAcquire("", nil, "id2", []string{"bar"}))
			},
			"IsReversible": func(t *testing.T, mngr ScopeManager) {
				require.NoError(t, mngr.ReleaseAndAcquire("id1", []string{"foo"}, "id2", []string{"bar"}))
				require.NoError(t, mngr.ReleaseAndAcquire("id2", []string{"bar"}, "id1", []string{"foo"}))
			},
			"SwapsScopesFully": func(t *testing.T, mngr ScopeManager) {
				require.NoError(t, mngr.Acquire("id1", []string{"foo"}))
				require.NoError(t, mngr.ReleaseAndAcquire("id1", []string{"foo"}, "id2", []string{"foo"}))
			},
			"SwapsScopesPartially": func(t *testing.T, mngr ScopeManager) {
				require.NoError(t, mngr.ReleaseAndAcquire("", nil, "id1", []string{"foo", "bar"}))
				require.NoError(t, mngr.ReleaseAndAcquire("id1", []string{"foo"}, "id2", []string{"foo"}))
				require.Error(t, mngr.Acquire("id2", []string{"bar"}))
			},
		} {
			t.Run(testName, func(t *testing.T) {
				testCase(t, NewLocalScopeManager())
			})
		}
	})
}
