package profitbricks

import (
	"github.com/dynport/dgtk/cli"
	"github.com/dynport/gocloud/profitbricks/actions"
)

func Register(router *cli.Router) {
	router.Register("pb/dcs/describe", &actions.DescribeDataCenterHandler{}, "Describe Data Center")
	router.RegisterFunc("pb/dcs/list", actions.ListAllDataCentersHandler, "List All DataCenters")
	router.Register("pb/servers/start", &actions.StartServer{}, "Start Server")
	router.Register("pb/servers/stop", &actions.StopServer{}, "Stop Server")
	router.Register("pb/servers/delete", &actions.DeleteServer{}, "Delete Server")
	router.RegisterFunc("pb/servers/list", actions.ListAllServersHandler, "List All Servers")
	router.Register("pb/servers/create", &actions.CreateServer{}, "Create Server")
	router.Register("pb/storages/delete", &actions.DeleteStorage{}, "Delete Storage")
	router.Register("pb/storages/create", &actions.CreateStorage{}, "Create Storage")
	router.RegisterFunc("pb/storages/list", actions.ListAllStorages, "List All Storages")
	router.RegisterFunc("pb/snapshots/list", actions.ListAllSnapshotsHandler, "List all snapshots")
	router.Register("pb/snapshots/rollback", &actions.RollbackSnapshotHandler{}, "Rollback Snapshot")
	router.RegisterFunc("pb/images/list", actions.ListAllImagesHandler, "List images")
}
