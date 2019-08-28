package host

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJasperRestartCounter(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, h *Host){
		"FailsIfNotInDatabase": func(t *testing.T, h *Host) {
			require.NoError(t, h.Remove())
			assert.Error(t, h.IncJasperRestartAttempts())
			assert.Error(t, h.SetJasperRestartAttempts(0))
		},
		"IncJasperRestartAttemptsSucceeds": func(t *testing.T, h *Host) {
			for i := 0; i < 10; i++ {
				require.NoError(t, h.IncJasperRestartAttempts())
				assert.Equal(t, i+1, h.JasperRestartAttempts)

				dbHost, err := FindOneId(h.Id)
				require.NoError(t, err)
				assert.Equal(t, i+1, dbHost.JasperRestartAttempts)
			}
		},
		"SetJasperRestartAttemptsSetsValue": func(t *testing.T, h *Host) {
			require.NoError(t, h.IncJasperRestartAttempts())
			assert.Equal(t, 1, h.JasperRestartAttempts)

			dbHost, err := FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, 1, dbHost.JasperRestartAttempts)

			attempts := 15
			require.NoError(t, h.SetJasperRestartAttempts(attempts))
			assert.Equal(t, attempts, h.JasperRestartAttempts)

			dbHost, err = FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, attempts, dbHost.JasperRestartAttempts)
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
