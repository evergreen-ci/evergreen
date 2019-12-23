package units

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
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

	collections := appendAmboyCollections(settings.Amboy, []backup.Options{})
	collections = appendFullBackupCollections(settings.Database.DB, collections)
	collections = appendInexOnlyBackupCollections(settings.Database.DB, collections)

	catcher := grip.NewBasicCatcher()
	for _, opt := range collections {
		catcher.Add(queue.Put(ctx, NewBackupMDBCollectionJob(opt, ts)))
	}

	return catcher.Resolve()
}

func appendInexOnlyBackupCollections(dbName string, in []backup.Options) []backup.Options {
	for _, coll := range []string{
		task.OldCollection,
		stats.HourlyTestStatsCollection,
		stats.DailyTaskStatsCollection,
		stats.DailyTestStatsCollection,
		stats.DailyStatsStatusCollection,
		model.TaskAliasQueuesCollection,
		model.TaskQueuesCollection,
		model.TestLogCollection,
		testresult.Collection,
		model.NotifyTimesCollection,
		model.NotifyHistoryCollection,
		event.AllLogCollection,
		event.SubscriptionsCollection,
		patch.IntentCollection,
	} {
		in = append(in, backup.Options{
			NS: amodel.Namespace{
				DB:         dbName,
				Collection: coll,
			},
			IndexesOnly: true,
		})
	}

	return in
}

func appendFullBackupCollections(dbName string, in []backup.Options) []backup.Options {
	for _, coll := range []string{
		evergreen.ConfigCollection,
		evergreen.CredentialsCollection, // grpc CA
		evergreen.ScopeCollection,       // acl data
		model.GithubHooksCollection,
		model.KeyValCollection,
		model.ProjectAliasCollection,
		model.ProjectRefCollection,
		model.ProjectVarsCollection,
		model.PushlogCollection,      // s3copy pushes
		model.RepositoriesCollection, // last seen hash
		user.Collection,
		db.GlobalsCollection, // revision_orderNumber
		distro.Collection,
		commitqueue.Collection,
		manifest.Collection,
	} {
		in = append(in, backup.Options{
			NS: amodel.Namespace{
				DB:         dbName,
				Collection: coll,
			},
			IndexesOnly: false,
		})
	}
	return in
}

func appendAmboyCollections(conf evergreen.AmboyConfig, in []backup.Options) []backup.Options {
	return append(in,
		backup.Options{
			NS: amodel.Namespace{
				DB:         conf.DB,
				Collection: conf.Name + ".jobs",
			},
			IndexesOnly: true,
		},
		backup.Options{
			NS: amodel.Namespace{
				DB:         conf.DB,
				Collection: conf.Name + ".group",
			},
			IndexesOnly: true,
		},
	)
}
