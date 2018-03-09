package eventtriggers

import (
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"gopkg.in/mgo.v2/bson"
)

func DoStuff(id string) (*task.Task, error) {
	_, _ = task.FindOne(task.ById(id))
	_, _ = patch.FindOne(patch.ById(bson.NewObjectId()))
	_, _ = version.FindOne(version.ById(id))
	_, _ = build.FindOne(build.ById(id))
	_, _ = host.FindOne(host.ById(id))
	return nil, nil
}
