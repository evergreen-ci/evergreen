package host

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJasperDeployCounter(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, h *Host){
		"FailsIfNotInDatabase": func(t *testing.T, h *Host) {
			require.NoError(t, h.Remove())
			assert.Error(t, h.IncJasperDeployAttempts())
			assert.Error(t, h.ResetJasperDeployAttempts())
		},
		"IncJasperDeployAttemptsSucceeds": func(t *testing.T, h *Host) {
			for i := 0; i < 10; i++ {
				require.NoError(t, h.IncJasperDeployAttempts())
				assert.Equal(t, i+1, h.JasperDeployAttempts)

				dbHost, err := FindOneId(h.Id)
				require.NoError(t, err)
				assert.Equal(t, i+1, dbHost.JasperDeployAttempts)
			}
		},
		"ResetJasperDeployAttemptsResetsToZero": func(t *testing.T, h *Host) {
			require.NoError(t, h.IncJasperDeployAttempts())
			assert.Equal(t, 1, h.JasperDeployAttempts)

			dbHost, err := FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, 1, dbHost.JasperDeployAttempts)

			require.NoError(t, h.ResetJasperDeployAttempts())
			assert.Zero(t, h.JasperDeployAttempts)

			dbHost, err = FindOneId(h.Id)
			require.NoError(t, err)
			assert.Zero(t, dbHost.JasperDeployAttempts)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection))
			defer func() {
				assert.NoError(t, db.ClearCollections(Collection))
			}()
			h := &Host{Id: "id"}
			require.NoError(t, h.Insert())
			testCase(t, h)
		})
	}
}
