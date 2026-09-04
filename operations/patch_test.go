package operations

import (
	"testing"

	"github.com/evergreen-ci/evergreen/rest/client"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestResolveDiffSource(t *testing.T) {
	patchID := primitive.NewObjectID().Hex()
	const mainlineVersionID = "sys_perf_54af8bc8daef529a87f01dba8dcc3a484ca910a3"
	const revision = "54af8bc8daef529a87f01dba8dcc3a484ca910a3"

	t.Run("PatchObjectIDShouldFetchRawPatch", func(t *testing.T) {
		comm := &client.Mock{
			GetRawPatchWithModulesResult: &restmodel.APIRawPatch{
				Patch: restmodel.APIRawModule{
					Diff:    "some diff",
					Githash: "abc123",
				},
				RawModules: []restmodel.APIRawModule{{Name: "module1", Diff: "module diff"}},
			},
		}
		var diffData localDiff
		rp, err := resolveDiffSource(t.Context(), comm, patchID, &diffData)
		require.NoError(t, err)
		require.NotNil(t, rp)
		assert.Equal(t, "some diff", diffData.fullPatch)
		assert.Equal(t, "abc123", diffData.base)
		// The raw patch is returned so the caller can replay module diffs onto the new patch.
		assert.Len(t, rp.RawModules, 1)
	})

	t.Run("MissingPatchShouldError", func(t *testing.T) {
		comm := &client.Mock{}
		var diffData localDiff
		rp, err := resolveDiffSource(t.Context(), comm, patchID, &diffData)
		assert.Nil(t, rp)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("MainlineVersionIDShouldSubmitEmptyDiffAtVersionRevision", func(t *testing.T) {
		comm := &client.Mock{
			GetVersionResult: &restmodel.APIVersion{
				Revision: utility.ToStringPtr(revision),
			},
			// A mainline ID must never reach the raw patch route, which rejects it with a 400.
			GetRawPatchWithModulesErr: errors.New("should not be called"),
		}
		var diffData localDiff
		rp, err := resolveDiffSource(t.Context(), comm, mainlineVersionID, &diffData)
		require.NoError(t, err)
		// A nil raw patch keeps the caller from trying to replay module diffs.
		assert.Nil(t, rp)
		assert.Empty(t, diffData.fullPatch)
		assert.Equal(t, revision, diffData.base)
	})

	t.Run("UnknownVersionIDShouldError", func(t *testing.T) {
		comm := &client.Mock{GetVersionErr: errors.New("404 version not found")}
		var diffData localDiff
		rp, err := resolveDiffSource(t.Context(), comm, mainlineVersionID, &diffData)
		assert.Nil(t, rp)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not a patch ID")
	})

	t.Run("VersionWithoutRevisionShouldError", func(t *testing.T) {
		comm := &client.Mock{GetVersionResult: &restmodel.APIVersion{}}
		var diffData localDiff
		rp, err := resolveDiffSource(t.Context(), comm, mainlineVersionID, &diffData)
		assert.Nil(t, rp)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no revision")
	})
}
