package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/anser/backup"
	amodel "github.com/mongodb/anser/model"
	"github.com/pkg/errors"
)

type backupCollectionHandler struct {
	collection string

	sc  data.Connector
	env evergreen.Environment
}

func makeBackupCollection(sc data.Connector, env evergreen.Environment) gimlet.RouteHandler {
	return &backupCollectionHandler{
		sc:  sc,
		env: env,
	}
}

func (b *backupCollectionHandler) Factory() gimlet.RouteHandler {
	return &backupCollectionHandler{
		sc:  b.sc,
		env: b.env,
	}
}

func (b *backupCollectionHandler) Parse(ctx context.Context, r *http.Request) error {
	b.collection = gimlet.GetVars(r)["collection"]
	if !b.env.Settings().Backup.Populated() {
		return errors.New("backup settings not populated")
	}
	if b.collection == "" {
		return errors.New("must pass collection name")
	}
	return nil
}

func (b *backupCollectionHandler) Run(ctx context.Context) gimlet.Responder {
	ctx = context.Background() // don't stop queue when web request finishes
	queue, err := b.env.RemoteQueueGroup().Get(ctx, "backup_collector_api")
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "problem getting remote queue group"))
	}
	opts := backup.Options{
		NS: amodel.Namespace{
			DB:         b.env.Settings().Database.DB,
			Collection: b.collection,
		},
	}
	if err := queue.Put(ctx, units.NewBackupMDBCollectionJob(opts, util.RoundPartOfHour(0))); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "problem putting job on queue"))
	}
	return gimlet.NewJSONResponse(struct{}{})
}
