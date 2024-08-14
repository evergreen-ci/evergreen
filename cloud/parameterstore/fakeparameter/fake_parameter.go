package fakeparameter

import (
	"context"
	"flag"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// ExecutionEnvironmentType is the type of environment in which the code is
// running. This exists a safety mechanism against accidentally calling this
// logic in non-testing environment. For tests, this should always be overridden
// to "test".
// kim: NOTE: can't do ClientType trick like for Secrets Manager because the
// production code will still import this package. May have to manually set PS
// implementation somehow. May be annoying to have to pass in PS implementation
// to all dependent code though. Possibly can have a compromise with something
// like Amboy queues pluggable implementations. Can set fake implementation in
// mock.Environment.Configure() (which is called manually in tests) and
// testutil.NewEnvironment() (which is init'd by _test.go files).
var ExecutionEnvironmentType = "production"

func init() {
	if ExecutionEnvironmentType != "test" {
		grip.EmergencyFatal(message.Fields{
			"message":     "fake Parameter Store testing code called in a non-testing environment",
			"environment": ExecutionEnvironmentType,
			"args":        flag.Args(),
		})
	}
}

// FakeParameter is the data model for a fake parameter stored in the DB. This
// is for testing only.
type FakeParameter struct {
	// ID is the unique identifying name for the parameter.
	ID string `bson:"_id,omitempty"`
	// Value is the parameter value.
	Value string `bson:"value,omitempty"`
	// LastUpdated is the last time the parameter was updated.
	LastUpdated time.Time `bson:"last_updated,omitempty"`
}

// Insert inserts a single parameter into the fake parameter store.
func (p *FakeParameter) Insert(ctx context.Context) error {
	_, err := evergreen.GetEnvironment().DB().Collection(Collection).InsertOne(ctx, p)
	return err
}
