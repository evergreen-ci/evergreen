package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/trigger"
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
		"JobShouldShareScopeWithDownstreamRepotracker": func(t *testing.T) {
			j := NewPushTriggerVersionsJob("msg", push, pushTriggered.Id)
			assert.Equal(t, NewRepotrackerJob("other-msg", pushTriggered.Id).Scopes(), j.Scopes())
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(model.ProjectRefCollection))
			t.Cleanup(func() {
				assert.NoError(t, db.ClearCollections(model.ProjectRefCollection))
			})
			require.NoError(t, pushTriggered.Insert(t.Context()))
			tCase(t)
		})
	}
}
