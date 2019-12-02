package jasper

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
	"github.com/tychoish/bond"
	"github.com/tychoish/bond/recall"
	"github.com/tychoish/lru"
)

func makeEnclosingDirectories(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		if err = os.MkdirAll(path, os.ModeDir|os.ModePerm); err != nil {
			return err
		}
	} else if !info.IsDir() {
		return errors.Errorf("'%s' already exists and is not a directory", path)
	}
	return nil
}

// SetupDownloadMongoDBReleases performs necessary setup to download MongoDB with the given options.
func SetupDownloadMongoDBReleases(ctx context.Context, cache *lru.Cache, opts options.MongoDBDownload) error {
	if err := makeEnclosingDirectories(opts.Path); err != nil {
		return errors.Wrap(err, "problem creating enclosing directories")
	}

	feed, err := bond.GetArtifactsFeed(ctx, opts.Path)
	if err != nil {
		return errors.Wrap(err, "problem making artifacts feed")
	}

	catcher := grip.NewBasicCatcher()
	urls, errs := feed.GetArchives(opts.Releases, opts.BuildOpts)
	jobs := createDownloadJobs(opts.Path, urls, catcher)

	if err = setupDownloadJobsAsync(ctx, jobs, processDownloadJobs(ctx, addMongoDBFilesToCache(cache, opts.Path))); err != nil {
		catcher.Add(errors.Wrap(err, "problem starting download jobs"))
	}

	for err = range errs {
		catcher.Add(errors.Wrap(err, "problem initializing download jobs"))
	}

	return catcher.Resolve()
}

func createDownloadJobs(path string, urls <-chan string, catcher grip.Catcher) <-chan amboy.Job {
	output := make(chan amboy.Job)

	go func() {
		for url := range urls {
			j, err := recall.NewDownloadJob(url, path, true)
			if err != nil {
				catcher.Add(errors.Wrapf(err, "problem creating download job for %s", url))
				continue
			}

			output <- j
		}
		close(output)
	}()

	return output
}

func processDownloadJobs(ctx context.Context, processFile func(string) error) func(amboy.Queue) error {
	return func(q amboy.Queue) error {
		grip.Infof("waiting for %d download jobs to complete", q.Stats(ctx).Total)
		if !amboy.WaitInterval(ctx, q, time.Second) {
			return errors.New("download job timed out")
		}
		grip.Info("all download tasks complete, processing errors now")

		if err := amboy.ResolveErrors(ctx, q); err != nil {
			return errors.Wrap(err, "problem completing download jobs")
		}

		catcher := grip.NewBasicCatcher()
		for job := range q.Results(ctx) {
			catcher.Add(job.Error())
			downloadJob, ok := job.(*recall.DownloadFileJob)
			if !ok {
				catcher.Add(errors.New("problem retrieving download job from queue"))
				continue
			}
			if err := processFile(filepath.Join(downloadJob.Directory, downloadJob.FileName)); err != nil {
				catcher.Add(err)
			}
		}
		return catcher.Resolve()
	}
}

func setupDownloadJobsAsync(ctx context.Context, jobs <-chan amboy.Job, processJobs func(amboy.Queue) error) error {
	q := queue.NewLocalLimitedSize(2, 1048)
	if err := q.Start(ctx); err != nil {
		return errors.Wrap(err, "problem starting download job queue")
	}

	if err := amboy.PopulateQueue(ctx, q, jobs); err != nil {
		return errors.Wrap(err, "problem adding download jobs to queue")
	}

	go func() {
		if err := processJobs(q); err != nil {
			grip.Errorf(errors.Wrap(err, "error occurred while adding jobs to cache").Error())
		}
	}()

	return nil
}

func addMongoDBFilesToCache(cache *lru.Cache, absRootPath string) func(string) error {
	return func(fileName string) error {
		filePath := filepath.Join(absRootPath, fileName)
		if err := cache.AddFile(filePath); err != nil {
			return errors.Wrapf(err, "problem adding file %s to cache", filePath)
		}

		baseName := filepath.Base(fileName)
		ext := filepath.Ext(baseName)
		dirPath := filepath.Join(absRootPath, strings.TrimSuffix(baseName, ext))

		err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Cache only handles individual files, not directories.
			if !info.IsDir() {
				if err := cache.AddStat(path, info); err != nil {
					return errors.Wrapf(err, "problem adding file %s to cache", path)
				}
			}

			return nil
		})
		return err
	}
}
