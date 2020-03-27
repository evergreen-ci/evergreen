package units

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/backup"
	amodel "github.com/mongodb/anser/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

func AddBackupJobs(ctx context.Context, env evergreen.Environment, ts time.Time) error {
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return errors.WithStack(err)
	}

	settings := env.Settings()

	if flags.DRBackupDisabled || !settings.Backup.Populated() {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"message":   "disaster recovery backups disabled or not configured",
			"impact":    "backup jobs not dispatched",
			"mode":      "disabled",
			"populated": settings.Backup.Populated(),
		})
		return nil
	}

	util.RoundPartOfDay(6)
	queue, err := env.RemoteQueueGroup().Get(ctx, "backup_collector")
	if err != nil {
		return errors.WithStack(err)
	}

	collections := appendAmboyCollections(settings.Amboy, []backup.Options{})
	collections = appendFullBackupCollections(settings.Database.DB, collections)
	collections = appendIndexOnlyBackupCollections(settings.Database.DB, collections)
	collections = appendQueryBackupCollection(settings.Database.DB, collections)

	catcher := grip.NewBasicCatcher()
	for _, opt := range collections {
		catcher.Add(queue.Put(ctx, NewBackupMDBCollectionJob(opt, ts)))
	}

	return catcher.Resolve()
}

func appendIndexOnlyBackupCollections(dbName string, in []backup.Options) []backup.Options {
	for _, coll := range []string{
		event.AllLogCollection,
		event.SubscriptionsCollection,
		model.TaskAliasQueuesCollection,
		model.TaskQueuesCollection,
		model.TestLogCollection,
		notification.Collection,
		patch.IntentCollection,
		stats.DailyStatsStatusCollection,
		stats.DailyTaskStatsCollection,
		stats.DailyTestStatsCollection,
		stats.HourlyTestStatsCollection,
		task.OldCollection,
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
		alertrecord.Collection,
		commitqueue.Collection,
		db.GlobalsCollection, // revision_orderNumber
		distro.Collection,
		evergreen.ConfigCollection,      // admin
		evergreen.CredentialsCollection, // grpc CA
		evergreen.RoleCollection,
		evergreen.ScopeCollection, // acl data
		manifest.Collection,
		model.GithubHooksCollection,
		model.KeyValCollection,
		model.NotesCollection,
		model.ProjectAliasCollection,
		model.ProjectRefCollection,
		model.ProjectVarsCollection,
		model.PushlogCollection,      // s3copy pushes
		model.RepositoriesCollection, // last seen hash
		model.TaskJSONCollection,
		user.Collection,
		host.VolumesCollection,
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

func appendQueryBackupCollection(dbName string, in []backup.Options) []backup.Options {
	for coll, createTime := range map[string]string{
		artifact.Collection:           artifact.CreateTimeKey,
		build.Collection:              build.CreateTimeKey,
		model.ParserProjectCollection: model.ParserProjectCreateTimeKey,
		patch.IntentCollection:        patch.CreateTimeKey,
		patch.Collection:              patch.CreateTimeKey,
		task.Collection:               task.CreateTimeKey,
		model.VersionCollection:       model.VersionCreateTimeKey,
	} {
		in = append(in, backup.Options{
			NS: amodel.Namespace{
				DB:         dbName,
				Collection: coll,
			},
			IndexesOnly: false,
			Query:       bson.M{createTime: bson.M{"$gt": time.Now().Add(30 * 24 * time.Hour)}},
		})
	}

	in = append(in, backup.Options{
		NS: amodel.Namespace{
			DB:         dbName,
			Collection: host.Collection,
		},
		IndexesOnly: false,
		Query:       bson.M{"$in": evergreen.UpHostStatus},
	})

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
