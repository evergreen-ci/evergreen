package command

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// computeCacheKey returns a hex-encoded SHA-256 over the contents of each key
// file followed by each expansion value, in the order given. Every entry is
// null-terminated so the key is order-sensitive and unambiguous: the same bytes
// split differently across entries hash to different keys. Nothing about the
// runtime environment is folded in implicitly; callers include values like
// "${distro_id}" through keyExpansions if they want that partitioning.
func computeCacheKey(keyFiles, keyExpansions []string) (string, error) {
	h := sha256.New()
	for _, keyFile := range keyFiles {
		f, err := os.Open(keyFile)
		if err != nil {
			return "", errors.Wrapf(err, "opening key file '%s'", keyFile)
		}
		_, err = io.Copy(h, f)
		closeErr := f.Close()
		if err != nil {
			return "", errors.Wrapf(err, "hashing key file '%s'", keyFile)
		}
		if closeErr != nil {
			return "", errors.Wrapf(closeErr, "closing key file '%s'", keyFile)
		}
		h.Write([]byte{0})
	}
	for _, value := range keyExpansions {
		h.Write([]byte(value))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// cacheHitExpansionName returns the name of the expansion that cache.restore
// sets and cache.save reads. Dashes in the cache name become underscores so the
// result is a usable shell variable name (e.g. "mise-and-go" -> "mise_and_go_cache_hit").
func cacheHitExpansionName(name string) string {
	return strings.ReplaceAll(name, "-", "_") + "_cache_hit"
}

// makeCacheArchive bundles paths (relative to workDir, or absolute) into a
// gzipped tarball at target. It reuses the same archive helpers as
// archive.targz_pack. A path that does not exist on disk is an error.
func makeCacheArchive(ctx context.Context, workDir string, paths []string, target string, logger grip.Journaler) error {
	contents, totalSize, err := gatherCacheContents(workDir, paths)
	if err != nil {
		return err
	}

	useParallelGzip := totalSize > thresholdSizeForParallelGzipCompression
	f, gz, tarWriter, err := tarGzWriter(target, useParallelGzip)
	if err != nil {
		return errors.Wrapf(err, "creating archive file '%s'", target)
	}
	defer func() {
		logger.Error(ctx, tarWriter.Close())
		logger.Error(ctx, gz.Close())
		logger.Error(ctx, f.Close())
	}()

	_, err = buildArchive(ctx, tarWriter, workDir, contents, nil, logger, false)
	return errors.Wrap(err, "building cache archive")
}

// gatherCacheContents resolves each path to the set of files to archive,
// walking directories recursively. Paths are interpreted relative to workDir
// unless absolute. A missing path is an error so a misconfigured cache.save
// fails loudly rather than uploading an incomplete archive.
func gatherCacheContents(workDir string, paths []string) ([]archiveContentFile, int, error) {
	var contents []archiveContentFile
	totalSize := 0
	for _, p := range paths {
		fullPath := p
		if !filepath.IsAbs(fullPath) {
			fullPath = filepath.Join(workDir, p)
		}

		info, err := os.Lstat(fullPath)
		if err != nil {
			return nil, 0, errors.Wrapf(err, "locating path '%s'", p)
		}

		if !info.IsDir() {
			contents = append(contents, archiveContentFile{path: fullPath, info: info})
			if info.Mode().IsRegular() {
				totalSize += int(info.Size())
			}
			continue
		}

		err = filepath.WalkDir(fullPath, func(walkPath string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			fi, err := d.Info()
			if err != nil {
				return errors.Wrapf(err, "getting file info for '%s'", walkPath)
			}
			contents = append(contents, archiveContentFile{path: walkPath, info: fi})
			if fi.Mode().IsRegular() {
				totalSize += int(fi.Size())
			}
			return nil
		})
		if err != nil {
			return nil, 0, errors.Wrapf(err, "walking directory '%s'", p)
		}
	}

	return contents, totalSize, nil
}
