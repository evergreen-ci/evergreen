package jasper

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mholt/archiver"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/tychoish/bond"
	"github.com/tychoish/bond/recall"
	"github.com/tychoish/lru"
)

// DownloadInfo represents the URL to download and the file path where it should be downloaded.
type DownloadInfo struct {
	URL         string         `json:"url"`
	Path        string         `json:"path"`
	ArchiveOpts ArchiveOptions `json:"archive_opts"`
}

// Validate checks the download options.
func (info DownloadInfo) Validate() error {
	catcher := grip.NewBasicCatcher()

	if !filepath.IsAbs(info.Path) {
		catcher.Add(errors.New("download path must be an absolute path"))
	}

	catcher.Add(info.ArchiveOpts.Validate())

	return catcher.Resolve()
}

// Download executes the download operation.
func (info DownloadInfo) Download() error {
	req, err := http.NewRequest(http.MethodGet, info.URL, nil)
	if err != nil {
		return errors.Wrap(err, "problem building request")
	}

	client := bond.GetHTTPClient()
	defer bond.PutHTTPClient(client)

	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrapf(err, "problem downloading file for url %s", info.URL)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("%s: could not download %s to path %s", resp.Status, info.URL, info.Path)
	}

	if err = writeFile(resp.Body, info.Path); err != nil {
		return err
	}

	if info.ArchiveOpts.ShouldExtract {
		if err = doExtract(info); err != nil {
			return errors.Wrapf(err, "problem extracting file %s to path %s", info.Path, info.ArchiveOpts.TargetPath)
		}
	}

	return nil
}

// ArchiveFormat represents an archive file type.
type ArchiveFormat string

const (
	// ArchiveAuto is an ArchiveFormat that does not force any particular type of
	// archive format.
	ArchiveAuto  ArchiveFormat = "auto"
	ArchiveTarGz               = "targz" // nolint
	ArchiveZip                 = "zip"   // nolint
)

// Validate checks that the ArchiveFormat is a recognized format.
func (f ArchiveFormat) Validate() error {
	switch f {
	case ArchiveTarGz, ArchiveZip, ArchiveAuto:
		return nil
	default:
		return errors.Errorf("unknown archive format %s", f)
	}
}

// ArchiveOptions encapsulates options related to management of archive files.
type ArchiveOptions struct {
	ShouldExtract bool
	Format        ArchiveFormat
	TargetPath    string
}

// Validate checks the archive file options.
func (opts ArchiveOptions) Validate() error {
	if !opts.ShouldExtract {
		return nil
	}

	catcher := grip.NewBasicCatcher()

	if !filepath.IsAbs(opts.TargetPath) {
		catcher.Add(errors.New("download path must be an absolute path"))
	}

	catcher.Add(opts.Format.Validate())

	return catcher.Resolve()
}

// MongoDBDownloadOptions represent one build variant of MongoDB.
type MongoDBDownloadOptions struct {
	BuildOpts bond.BuildOptions `json:"build_opts"`
	Path      string            `json:"path"`
	Releases  []string          `json:"releases"`
}

// Validate checks for valid MongoDB download options.
func (opts MongoDBDownloadOptions) Validate() error {
	catcher := grip.NewBasicCatcher()

	if !filepath.IsAbs(opts.Path) {
		catcher.Add(errors.New("download path must be an absolute path"))
	}

	catcher.Add(opts.BuildOpts.Validate())

	return catcher.Resolve()
}

// CacheOptions represent the configuration options for the LRU cache.
type CacheOptions struct {
	Disabled   bool          `json:"disabled"`
	PruneDelay time.Duration `json:"prune_delay"`
	MaxSize    int           `json:"max_size"`
}

// Validate checks for valid cache options.
func (opts CacheOptions) Validate() error {
	catcher := grip.NewBasicCatcher()

	if opts.MaxSize < 0 {
		catcher.Add(errors.New("max size cannot be negative"))
	}

	if opts.PruneDelay < 0 {
		catcher.Add(errors.New("prune delay cannot be negative"))
	}

	return catcher.Resolve()
}

func doExtract(info DownloadInfo) error {
	var archiveHandler archiver.Archiver
	switch info.ArchiveOpts.Format {
	case ArchiveAuto:
		unzipper := archiver.MatchingFormat(info.Path)
		if unzipper == nil {
			return errors.Errorf("could not detect archive format for %s", info.Path)
		}
		archiveHandler = unzipper
	case ArchiveTarGz:
		archiveHandler = archiver.TarGz
	case ArchiveZip:
		archiveHandler = archiver.Zip
	default:
		return errors.Errorf("unrecognized archive format %s", info.ArchiveOpts.Format)
	}

	if err := archiveHandler.Open(info.Path, info.ArchiveOpts.TargetPath); err != nil {
		return errors.Wrapf(err, "problem extracting archive %s to %s", info.Path, info.ArchiveOpts.TargetPath)
	}

	return nil
}

// SetupDownloadMongoDBReleases performs necessary setup to download MongoDB with the given options.
func SetupDownloadMongoDBReleases(ctx context.Context, cache *lru.Cache, opts MongoDBDownloadOptions) error {
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

	if err := setupDownloadJobsAsync(ctx, jobs, processDownloadJobs(ctx, addMongoDBFilesToCache(cache, opts.Path))); err != nil {
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
		grip.Infof("waiting for %d download jobs to complete", q.Stats().Total)
		_ = amboy.WaitCtxInterval(ctx, q, 1000*time.Millisecond)
		grip.Info("all download tasks complete, processing errors now")

		if err := amboy.ResolveErrors(ctx, q); err != nil {
			return errors.Wrap(err, "problem completing download jobs")
		}

		catcher := grip.NewBasicCatcher()
		for job := range q.Results(ctx) {
			downloadJob, ok := job.(*recall.DownloadFileJob)
			if !ok {
				catcher.Add(errors.New("problem retrieving download job from queue"))
				continue
			}
			if err := processFile(downloadJob.FileName); err != nil {
				catcher.Add(err)
			}
		}
		return catcher.Resolve()
	}
}

func setupDownloadJobsAsync(ctx context.Context, jobs <-chan amboy.Job, processJobs func(amboy.Queue) error) error {
	q := queue.NewLocalUnordered(2)
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
