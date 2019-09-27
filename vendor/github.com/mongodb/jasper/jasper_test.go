package jasper

import (
	"context"

	"github.com/mongodb/grip"
	"github.com/mongodb/jasper/options"
)

func makeLockingProcess(pmake ProcessConstructor) ProcessConstructor {
	return func(ctx context.Context, opts *options.Create) (Process, error) {
		proc, err := pmake(ctx, opts)
		if err != nil {
			return nil, err
		}
		return &localProcess{proc: proc}, nil
	}
}

func createProcs(ctx context.Context, opts *options.Create, manager Manager, num int) ([]Process, error) {
	catcher := grip.NewBasicCatcher()
	out := []Process{}
	for i := 0; i < num; i++ {
		optsCopy := *opts

		proc, err := manager.CreateProcess(ctx, &optsCopy)
		catcher.Add(err)
		if proc != nil {
			out = append(out, proc)
		}
	}

	return out, catcher.Resolve()
}
