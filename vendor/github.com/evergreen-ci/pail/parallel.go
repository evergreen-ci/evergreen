package pail

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type parallelBucketImpl struct {
	Bucket
	size         int
	deleteOnPush bool
	deleteOnPull bool
	dryRun       bool
}

// ParallelBucketOptions support the use and creation of parallel sync buckets.
type ParallelBucketOptions struct {
	// Workers sets the number of worker threads.
	Workers int
	// DryRun enables running in a mode that will not execute any
	// operations that modify the bucket.
	DryRun bool
	// DeleteOnSync will delete all objects from the target that do not
	// exist in the source after the completion of a sync operation
	// (Push/Pull).
	DeleteOnSync bool
	// DeleteOnPush will delete all objects from the target that do not
	// exist in the source after the completion of Push.
	DeleteOnPush bool
	// DeleteOnPull will delete all objects from the target that do not
	// exist in the source after the completion of Pull.
	DeleteOnPull bool
}

// NewParallelSyncBucket returns a layered bucket implemenation that supports
// parallel sync operations.
func NewParallelSyncBucket(opts ParallelBucketOptions, b Bucket) (Bucket, error) {
	if (opts.DeleteOnPush != opts.DeleteOnPull) && opts.DeleteOnSync {
		return nil, errors.New("ambiguous delete on sync options set")
	}

	return &parallelBucketImpl{
		size:         opts.Workers,
		deleteOnPush: opts.DeleteOnPush || opts.DeleteOnSync,
		deleteOnPull: opts.DeleteOnPull || opts.DeleteOnSync,
		dryRun:       opts.DryRun,
		Bucket:       b,
	}, nil
}

func (b *parallelBucketImpl) Push(ctx context.Context, opts SyncOptions) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var re *regexp.Regexp
	var err error
	if opts.Exclude != "" {
		re, err = regexp.Compile(opts.Exclude)
		if err != nil {
			return errors.Wrap(err, "problem compiling exclude regex")
		}
	}

	files, err := walkLocalTree(ctx, opts.Local)
	if err != nil {
		return errors.WithStack(err)
	}

	in := make(chan string, len(files))
	for i := range files {
		if re != nil && re.MatchString(files[i]) {
			continue
		}
		in <- files[i]
	}
	close(in)
	wg := &sync.WaitGroup{}
	catcher := grip.NewBasicCatcher()
	for i := 0; i < b.size; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for fn := range in {
				select {
				case <-ctx.Done():
					return
				default:
				}

				if b.dryRun {
					continue
				}

				if err := b.Bucket.Upload(ctx, filepath.Join(opts.Remote, fn), filepath.Join(opts.Local, fn)); err != nil {
					catcher.Add(err)
					cancel()
				}
			}
		}()
	}
	wg.Wait()

	if ctx.Err() == nil && b.deleteOnPush && !b.dryRun {
		catcher.Add(errors.Wrap(deleteOnPush(ctx, files, opts.Remote, b), "problem with delete on sync after push"))
	}

	return catcher.Resolve()

}
func (b *parallelBucketImpl) Pull(ctx context.Context, opts SyncOptions) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var re *regexp.Regexp
	var err error
	if opts.Exclude != "" {
		re, err = regexp.Compile(opts.Exclude)
		if err != nil {
			return errors.Wrap(err, "problem compiling exclude regex")
		}
	}

	iter, err := b.List(ctx, opts.Remote)
	if err != nil {
		return errors.WithStack(err)
	}

	catcher := grip.NewBasicCatcher()
	items := make(chan BucketItem)
	toDelete := make(chan string)

	go func() {
		defer close(items)

		for iter.Next(ctx) {
			if iter.Err() != nil {
				cancel()
				catcher.Add(errors.Wrap(iter.Err(), "problem iterating bucket"))
			}

			if re != nil && re.MatchString(iter.Item().Name()) {
				continue
			}

			select {
			case <-ctx.Done():
				catcher.Add(ctx.Err())
				return
			case items <- iter.Item():
			}
		}
	}()

	wg := &sync.WaitGroup{}
	for i := 0; i < b.size; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range items {
				name, err := filepath.Rel(opts.Remote, item.Name())
				if err != nil {
					catcher.Add(errors.Wrap(err, "problem getting relative filepath"))
					cancel()
				}
				localName := filepath.Join(opts.Local, name)
				if err := b.Download(ctx, item.Name(), localName); err != nil {
					catcher.Add(err)
					cancel()
				}

				fn := strings.TrimPrefix(item.Name(), opts.Remote)
				fn = strings.TrimPrefix(fn, "/")
				fn = strings.TrimPrefix(fn, "\\") // cause windows...

				select {
				case <-ctx.Done():
					catcher.Add(ctx.Err())
					return
				case toDelete <- fn:
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(toDelete)
	}()

	deleteSignal := make(chan struct{})
	go func() {
		defer close(deleteSignal)

		keys := []string{}
		for key := range toDelete {
			keys = append(keys, key)
		}

		if b.deleteOnPull && b.dryRun {
			grip.Debug(message.Fields{
				"dry_run": true,
				"message": "would delete after push",
			})
		} else if ctx.Err() == nil && b.deleteOnPull {
			catcher.Add(errors.Wrap(deleteOnPull(ctx, keys, opts.Local), "problem with delete on sync after pull"))
		}
	}()

	select {
	case <-ctx.Done():
	case <-deleteSignal:
	}

	return catcher.Resolve()
}
