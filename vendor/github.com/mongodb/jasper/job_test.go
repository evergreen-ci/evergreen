package jasper

import (
	"context"
	"strings"
	"testing"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAmboyJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("Registry", func(t *testing.T) {
		count := 0
		for n := range registry.JobTypeNames() {
			if n == "bond-recall-download-file" {
				continue
			}
			if n != "" {
				count++
			}
		}
		require.Equal(t, 0, count)

		RegisterJobs(newBasicProcess)

		for n := range registry.JobTypeNames() {
			if n == "bond-recall-download-file" {
				continue
			}
			if n != "" {
				assert.Contains(t, n, "jasper")
				count++
			}
		}
		require.Equal(t, 3, count)
	})
	t.Run("RoundTripMarshal", func(t *testing.T) {
		for _, frm := range []amboy.Format{amboy.BSON2, amboy.JSON} {
			t.Run(strings.ToUpper(frm.String()), func(t *testing.T) {
				for _, j := range []amboy.Job{
					NewJob(newBasicProcess, "ls"),
					NewJobOptions(newBasicProcess, &CreateOptions{}),
					NewJobForeground(newBasicProcess, &CreateOptions{}),
				} {
					ic, err := registry.MakeJobInterchange(j, frm)
					require.NoError(t, err)
					require.NotNil(t, ic)

					j2, err := ic.Resolve(frm)
					require.NoError(t, err)
					require.Equal(t, j2.Type(), j.Type())
				}
			})
		}
	})
	t.Run("TypeCheck", func(t *testing.T) {
		t.Run("Default", func(t *testing.T) {
			job, ok := NewJob(newBasicProcess, "ls").(*amboyJob)
			assert.True(t, ok)
			assert.NotNil(t, job)
		})
		t.Run("DefaultBasic", func(t *testing.T) {
			job, ok := NewJobBasic("ls").(*amboyJob)
			assert.True(t, ok)
			assert.NotNil(t, job)
		})
		t.Run("Simple", func(t *testing.T) {
			job, ok := NewJobOptions(newBasicProcess, &CreateOptions{}).(*amboySimpleCapturedOutputJob)
			assert.True(t, ok)
			assert.NotNil(t, job)
		})
		t.Run("Foreground", func(t *testing.T) {
			job, ok := NewJobForeground(newBasicProcess, &CreateOptions{}).(*amboyForegroundOutputJob)
			assert.True(t, ok)
			assert.NotNil(t, job)
		})
		t.Run("ForegroundBasic", func(t *testing.T) {
			job, ok := NewJobBasicForeground(&CreateOptions{}).(*amboyForegroundOutputJob)
			assert.True(t, ok)
			assert.NotNil(t, job)
		})
	})
	t.Run("BasicExec", func(t *testing.T) {
		t.Run("Default", func(t *testing.T) {
			job := NewJob(newBasicProcess, "ls")
			job.Run(ctx)
			require.NoError(t, job.Error())
		})
		t.Run("DefaultBasic", func(t *testing.T) {
			job := NewJobBasic("ls")
			job.Run(ctx)
			require.NoError(t, job.Error())
		})
		t.Run("Simple", func(t *testing.T) {
			job := NewJobOptions(newBasicProcess, &CreateOptions{Args: []string{"echo", "hi"}})
			job.Run(ctx)
			require.NoError(t, job.Error())
		})
		t.Run("Foreground", func(t *testing.T) {
			job := NewJobForeground(newBasicProcess, &CreateOptions{Args: []string{"echo", "hi"}})
			job.Run(ctx)
			require.NoError(t, job.Error())
		})
	})
	t.Run("ReExecErrors", func(t *testing.T) {
		t.Run("Default", func(t *testing.T) {
			job := NewJob(newBasicProcess, "ls")
			job.Run(ctx)
			require.NoError(t, job.Error())
			job.Run(ctx)
			require.Error(t, job.Error())
		})
		t.Run("Simple", func(t *testing.T) {
			job := NewJobOptions(newBasicProcess, &CreateOptions{Args: []string{"echo", "hi"}})
			job.Run(ctx)
			require.NoError(t, job.Error())
			job.Run(ctx)
			require.Error(t, job.Error())
		})
		t.Run("Foreground", func(t *testing.T) {
			job := NewJobForeground(newBasicProcess, &CreateOptions{Args: []string{"echo", "hi"}})
			job.Run(ctx)
			require.NoError(t, job.Error())
			job.Run(ctx)
			require.Error(t, job.Error())
		})
	})
}
