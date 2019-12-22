package units

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/backup"
	amodel "github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
)

func AddBackupJobs(ctx context.Context, env evergreen.Environment, ts time.Time) error {
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return errors.WithStack(err)
	}

	if flags.DRBackupDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"message": "disaster recovery backups disabled",
			"impact":  "backup jobs not dispatched",
			"mode":    "degraded",
		})
		return nil
	}

	util.RoundPartOfDay(6)
	queue, err := env.RemoteQueueGroup().Get(ctx, "backup_collector")
	if err != nil {
		return errors.WithStack(err)
	}
	settings := env.Settings()
	catcher := grip.NewBasicCatcher()
	for _, opt := range []backup.Options{
		{
			NS: amodel.Namespace{
				DB:         settings.Database.DB,
				Collection: model.ProjectRefCollection,
			},
		},
		{
			NS: amodel.Namespace{
				DB:         settings.Database.DB,
				Collection: model.ProjectVarsCollection,
			},
		},
		{
			NS: amodel.Namespace{
				DB:         settings.Database.DB,
				Collection: model.ProjectAliasCollection,
			},
		},
		{
			NS: amodel.Namespace{
				DB:         settings.Database.DB,
				Collection: evergreen.ConfigCollection,
			},
		},
		{
			NS: amodel.Namespace{
				DB:         settings.Database.DB,
				Collection: evergreen.ScopeCollection,
			},
		},
		{
			NS: amodel.Namespace{
				// s3copy pushes
				DB:         settings.Database.DB,
				Collection: model.PushlogCollection,
			},
		},
		{
			NS: amodel.Namespace{
				// revision_order_number
				DB:         settings.Database.DB,
				Collection: model.RepositoriesCollection,
			},
		},
		{
			NS: amodel.Namespace{
				// revision_order_number
				DB:         settings.Database.DB,
				Collection: db.GlobalsCollection,
			},
		},
		{
			NS: amodel.Namespace{
				DB:         settings.Database.DB,
				Collection: model.GithubHooksCollection,
			},
		},
		{
			NS: amodel.Namespace{
				DB:         settings.Database.DB,
				Collection: distro.Collection,
			},
		},
		{
			NS: amodel.Namespace{
				// grpc credentials
				// TODO: ttl and/or filter this by
				// running hosts?
				DB:         settings.Database.DB,
				Collection: evergreen.CredentialsCollection,
			},
		},
		{
			NS: amodel.Namespace{
				DB:         settings.Database.DB,
				Collection: user.Collection,
			},
		},
		{
			NS: amodel.Namespace{
				DB:         settings.Database.DB,
				Collection: model.KeyValCollection,
			},
		},
		{
			NS: amodel.Namespace{
				DB:         settings.Amboy.DB,
				Collection: settings.Amboy.Name + ".jobs",
			},
			IndexesOnly: true,
		},
		{
			NS: amodel.Namespace{
				DB:         settings.Amboy.DB,
				Collection: settings.Amboy.Name + ".group",
			},
			IndexesOnly: true,
		},
	} {
		catcher.Add(queue.Put(ctx, NewBackupMDBCollectionJob(opt, ts)))
	}

	return catcher.Resolve()
}
