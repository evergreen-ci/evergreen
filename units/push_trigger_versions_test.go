package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/trigger"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPushTriggerVersionsJob(t *testing.T) {
	const upstreamProjectID = "upstream"
	push := trigger.UpstreamPush{ProjectID: upstreamProjectID, Commits: []model.Revision{{Revision: "sha"}}, IngestTime: time.Now()}
	pushTriggered := model.ProjectRef{
		Id:         "push-triggered",
		Identifier: "push-triggered",
		Enabled:    true,
		Triggers:   []model.TriggerDefinition{{Project: upstreamProjectID, Level: model.ProjectTriggerLevelPush, DefinitionID: "def"}},
	}
	taskTriggered := model.ProjectRef{
		Id:         "task-triggered",
		Identifier: "task-triggered",
		Enabled:    true,
		Triggers:   []model.TriggerDefinition{{Project: upstreamProjectID, Level: model.ProjectTriggerLevelTask, DefinitionID: "def"}},
	}

	for tName, tCase := range map[string]func(t *testing.T){
		"PushTriggeredProjectShouldGetVersion": func(t *testing.T) {
			var triggered []string
			j := NewPushTriggerVersionsJob("msg", push, pushTriggered.Id).(*pushTriggerVersionsJob)
			j.processor = func(_ context.Context, args trigger.ProcessorArgs) (*model.Version, error) {
				triggered = append(triggered, args.DownstreamProject.Id+"@"+args.PushRevision.Revision)
				return &model.Version{Id: "v"}, nil
			}

			j.Run(t.Context())
			require.NoError(t, j.Error())
			assert.Equal(t, []string{"push-triggered@sha"}, triggered)
		},
		"ProjectWithoutPushTriggerShouldNotGetVersion": func(t *testing.T) {
			j := NewPushTriggerVersionsJob("msg", push, taskTriggered.Id).(*pushTriggerVersionsJob)
			j.processor = func(context.Context, trigger.ProcessorArgs) (*model.Version, error) {
				assert.Fail(t, "project without a push trigger should not get a version")
				return nil, nil
			}

			j.Run(t.Context())
			assert.NoError(t, j.Error())
		},
		"FailedVersionShouldBeRetried": func(t *testing.T) {
			j := NewPushTriggerVersionsJob("msg", push, pushTriggered.Id).(*pushTriggerVersionsJob)
			j.processor = func(context.Context, trigger.ProcessorArgs) (*model.Version, error) {
				return nil, errors.New("transient GitHub error")
			}

			j.Run(t.Context())
			assert.Error(t, j.Error())
			assert.True(t, j.RetryInfo().ShouldRetry())
		},
		"JobShouldShareScopeWithDownstreamRepotracker": func(t *testing.T) {
			j := NewPushTriggerVersionsJob("msg", push, pushTriggered.Id)
			assert.Equal(t, NewRepotrackerJob("other-msg", pushTriggered.Id).Scopes(), j.Scopes())
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.VersionCollection))
			t.Cleanup(func() {
				assert.NoError(t, db.ClearCollections(model.ProjectRefCollection, model.VersionCollection))
			})
			require.NoError(t, pushTriggered.Insert(t.Context()))
			require.NoError(t, taskTriggered.Insert(t.Context()))
			tCase(t)
		})
	}
}
