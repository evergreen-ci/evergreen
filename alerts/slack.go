package alerts

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type slackDeliverer struct {
	logger grip.Journaler
	uiRoot string
}

func (s *slackDeliverer) Deliver(ctx AlertContext, conf model.AlertConfig) error {
	description, err := getDescription(ctx, s.uiRoot)
	if err != nil {
		return errors.WithStack(err)
	}

	s.logger.Notice(message.Fields{
		"message":     getSummary(ctx),
		"tasks":       ctx.Task.DisplayName,
		"variant":     ctx.Task.BuildVariant,
		"project":     ctx.ProjectRef.Identifier,
		"description": description,
	})

	return nil
}
