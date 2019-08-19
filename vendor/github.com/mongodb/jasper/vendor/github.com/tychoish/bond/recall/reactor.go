package recall

import (
	"context"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/tychoish/bond"
)

// DownloadReleases accesses the feed and, based on the arguments
// provided, does the work to download the specified versions of
// MongoDB from the downloads feed. This operation is meant to provide
// the basis for command-line interfaces for downloading groups of
// MongoDB versions.
func DownloadReleases(releases []string, path string, options bond.BuildOptions) error {
	return FetchReleases(context.Background(), releases, path, options)
}

// FetchReleases has the same behavior as DownloadReleases, but takes
// a Context to facilitate caller implemented timeouts and cancelation.
func FetchReleases(ctx context.Context, releases []string, path string, options bond.BuildOptions) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	if err := options.Validate(); err != nil {
		return errors.Wrap(err, "invalid build options")
	}

	feed, err := bond.GetArtifactsFeed(ctx, path)
	if err != nil {
		return errors.Wrap(err, "problem generating data feed")
	}

	q := queue.NewLocalUnordered(4)
	if err := q.Start(ctx); err != nil {
		return errors.Wrap(err, "problem starting queue")
	}

	urls, errGroupOne := feed.GetArchives(releases, options)
	jobs, errGroupTwo := createJobs(path, urls)

	if err := amboy.PopulateQueue(ctx, q, jobs); err != nil {
		return errors.Wrap(err, "problem adding jobs to queue")
	}

	if err := aggregateErrors(errGroupOne, errGroupTwo); err != nil {
		return errors.Wrap(err, "problem populating jobs")
	}

	grip.Debugf("waiting for %d download jobs to complete", q.Stats(ctx).Total)
	amboy.WaitInterval(ctx, q, 100*time.Millisecond)
	grip.Debug("all download tasks complete, processing errors now")

	if err := amboy.ResolveErrors(ctx, q); err != nil {
		return errors.Wrap(err, "problem(s) detected in download jobs")
	}

	return nil
}

func createJobs(path string, urls <-chan string) (<-chan amboy.Job, <-chan error) {
	output := make(chan amboy.Job)
	errOut := make(chan error)

	go func() {
		catcher := grip.NewCatcher()
		for url := range urls {
			j, err := NewDownloadJob(url, path, false)
			if err != nil {
				catcher.Add(errors.Wrapf(err,
					"problem generating task for %s", url))
				continue
			}

			output <- j
		}
		close(output)
		if catcher.HasErrors() {
			errOut <- catcher.Resolve()
		}
		close(errOut)
	}()

	return output, errOut
}

func aggregateErrors(groups ...<-chan error) error {
	catcher := grip.NewCatcher()

	for _, g := range groups {
		for err := range g {
			catcher.Add(err)
		}
	}

	return catcher.Resolve()
}
