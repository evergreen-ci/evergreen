package util

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

func ParallelWorkerExec[T any](ctx context.Context, name string, work []T, logger grip.Journaler, handler func(item *T) error) (int64, error) {
	wc := make(chan *T, len(work))
	for i := range work {
		wc <- &work[i]
	}
	close(wc)
	var succeeded int64
	eg, ctx := errgroup.WithContext(ctx)
	for range runtime.GOMAXPROCS(0) {
		eg.Go(func() error {
			defer func() {
				logger.Critical(recovery.HandlePanicWithError(recover(), nil, fmt.Sprintf("%s worker", name)))
			}()
			for item := range wc {
				if err := ctx.Err(); err != nil {
					return errors.Wrap(err, fmt.Sprintf("canceled while handling item for %s", name))
				}
				if err := handler(item); err != nil {
					// Continue on error to let the other logs get posted.
					logger.Error(errors.Wrap(err, name))
					continue
				}
				atomic.AddInt64(&succeeded, 1)

				// Yield to allow other goroutines to run and prevent starvation
				// in intense log uploading workflows.
				runtime.Gosched()
			}
			return nil
		})
	}

	return succeeded, eg.Wait()
}
