package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"context"

	humanize "github.com/dustin/go-humanize"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const defaultCloneDepth = 500

// FetchCommand is used to fetch the source or artifacts associated with a task.
type FetchCommand struct {
	GlobalOpts *Options `no-flag:"true"`

	Source    bool   `long:"source" description:"clones the source for the given task"`
	Artifacts bool   `long:"artifacts" description:"fetch artifacts for the task and all its recursive dependents"`
	Shallow   bool   `long:"shallow" description:"don't recursively download artifacts from dependency tasks"`
	NoPatch   bool   `long:"no-patch" description:"when using --source with a patch task, skip applying the patch"`
	Dir       string `long:"dir" description:"root directory to fetch artifacts into. defaults to current working directory"`
	TaskId    string `short:"t" long:"task" description:"task associated with the data to fetch" required:"true"`
}

// FetchCommand allows the user to download the artifacts for a task (and optionally its dependencies),
// clone the source that a task was derived from, or both.
func (fc *FetchCommand) Execute(_ []string) error {
	ctx := context.Background()
	ac, rc, _, err := getAPIClients(ctx, fc.GlobalOpts)
	if err != nil {
		return err
	}
	notifyUserUpdate(ac)

	wd := fc.Dir
	if len(wd) == 0 {
		wd, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	if len(fc.TaskId) == 0 {
		return errors.Errorf("must specify a task ID with -t.")
	}

	if !fc.Source && !fc.Artifacts {
		return errors.New("must specify at least one of either --artifacts or --source.")
	}
	if fc.Source {
		err = fetchSource(ac, rc, wd, fc.TaskId, fc.NoPatch)
		if err != nil {
			return err
		}
	}
	if fc.Artifacts {
		err = fetchArtifacts(rc, fc.TaskId, wd, fc.Shallow)
		if err != nil {
			return err
		}
	}
	return nil
}

func fetchSource(ac, rc *APIClient, rootPath, taskId string, noPatch bool) error {
	task, err := rc.GetTask(taskId)
	if err != nil {
		return err
	}
	if task == nil {
		return errors.New("task not found.")
	}

	config, err := rc.GetConfig(task.Version)
	if err != nil {
		return err
	}

	project, err := ac.GetProjectRef(task.Project)
	if err != nil {
		return err
	}

	cloneDir := util.CleanForPath(fmt.Sprintf("source-%v", task.Project))
	var patch *service.RestPatch
	if evergreen.IsPatchRequester(task.Requester) {
		cloneDir = util.CleanForPath(fmt.Sprintf("source-patch-%v_%v", task.PatchNumber, task.Project))
		patch, err = rc.GetPatch(task.PatchId)
		if err != nil {
			return err
		}
	} else {
		if len(task.Revision) >= 5 {
			cloneDir = util.CleanForPath(fmt.Sprintf("source-%v-%v", task.Project, task.Revision[0:6]))
		}
	}
	cloneDir = filepath.Join(rootPath, cloneDir)

	err = cloneSource(task, project, config, cloneDir)
	if err != nil {
		return err
	}
	if patch != nil && !noPatch {
		err = applyPatch(patch, cloneDir, config, config.FindBuildVariant(task.BuildVariant))
		if err != nil {
			return err
		}
	}

	return nil
}

type cloneOptions struct {
	repo     string
	revision string
	rootDir  string
	depth    uint
}

func clone(opts cloneOptions) error {
	// clone the repo first
	cloneArgs := []string{"clone", opts.repo}
	if opts.depth > 0 {
		cloneArgs = append(cloneArgs, "--depth", fmt.Sprintf("%d", opts.depth))
	}

	cloneArgs = append(cloneArgs, opts.rootDir)
	grip.Debug(cloneArgs)

	c := exec.Command("git", cloneArgs...)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	err := c.Run()
	if err != nil {
		return err
	}

	// try to check out the revision we want
	checkoutArgs := []string{"checkout", opts.revision}
	grip.Debug(checkoutArgs)

	c = exec.Command("git", checkoutArgs...)
	stdoutBuf, stderrBuf := &bytes.Buffer{}, &bytes.Buffer{}
	c.Stdout = io.MultiWriter(os.Stdout, stdoutBuf)
	c.Stderr = io.MultiWriter(os.Stderr, stderrBuf)
	c.Dir = opts.rootDir
	err = c.Run()
	if err != nil {
		if !bytes.Contains(stderrBuf.Bytes(), []byte("reference is not a tree:")) {
			return err
		}

		// we have to go deeper
		fetchArgs := []string{"fetch", "--unshallow"}
		grip.Debug(fetchArgs)

		c = exec.Command("git", fetchArgs...)
		c.Stdout, c.Stderr, c.Dir = os.Stdout, os.Stderr, opts.rootDir
		err = c.Run()
		if err != nil {
			return err
		}
		// now it's unshallow, so try again to check it out
		checkoutRetryArgs := []string{"checkout", opts.revision}
		grip.Debug(checkoutRetryArgs)

		c = exec.Command("git", checkoutRetryArgs...)
		c.Stdout, c.Stderr, c.Dir = os.Stdout, os.Stderr, opts.rootDir
		return c.Run()
	}
	return nil
}

func cloneSource(task *service.RestTask, project *model.ProjectRef, config *model.Project, cloneDir string) error {
	// Fetch the outermost repo for the task
	err := clone(cloneOptions{
		repo:     fmt.Sprintf("git@github.com:%v/%v.git", project.Owner, project.Repo),
		revision: task.Revision,
		rootDir:  cloneDir,
		depth:    defaultCloneDepth,
	})

	if err != nil {
		return err
	}

	// Then fetch each of the modules
	variant := config.FindBuildVariant(task.BuildVariant)
	if variant == nil {
		return errors.Errorf("couldn't find build variant '%v' in config", task.BuildVariant)
	}
	for _, moduleName := range variant.Modules {
		module, err := config.GetModuleByName(moduleName)
		if err != nil || module == nil {
			return errors.Errorf("variant refers to a module '%v' that doesn't exist.", moduleName)
		}
		moduleBase := filepath.Join(cloneDir, module.Prefix, module.Name)
		fmt.Printf("Fetching module %v at %v\n", moduleName, module.Branch)
		err = clone(cloneOptions{
			repo:     module.Repo,
			revision: module.Branch,
			rootDir:  filepath.ToSlash(moduleBase),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func applyPatch(patch *service.RestPatch, rootCloneDir string, conf *model.Project, variant *model.BuildVariant) error {
	// patch sets and contain multiple patches, some of them for modules
	for _, patchPart := range patch.Patches {
		var dir string
		if patchPart.ModuleName == "" {
			// if patch is not part of a module, just apply patch against src root
			dir = rootCloneDir
		} else {
			fmt.Println("Applying patches for module", patchPart.ModuleName)
			// if patch is part of a module, apply patch in module root
			module, err := conf.GetModuleByName(patchPart.ModuleName)
			if err != nil || module == nil {
				return errors.Errorf("can't find module %v: %v", patchPart.ModuleName, err)
			}

			// skip the module if this build variant does not use it
			if !util.StringSliceContains(variant.Modules, module.Name) {
				continue
			}

			dir = filepath.Join(rootCloneDir, module.Prefix, module.Name)
		}

		args := []string{"apply", "--whitespace=fix"}
		applyCmd := exec.Command("git", args...)
		applyCmd.Stdout, applyCmd.Stderr, applyCmd.Dir = os.Stdout, os.Stderr, dir
		applyCmd.Stdin = bytes.NewReader([]byte(patchPart.PatchSet.Patch))
		err := applyCmd.Run()
		if err != nil {
			return err
		}
	}
	return nil
}

func fetchArtifacts(rc *APIClient, taskId string, rootDir string, shallow bool) error {
	task, err := rc.GetTask(taskId)
	if err != nil {
		return errors.Wrapf(err, "problem getting task for %s", taskId)
	}
	if task == nil {
		return errors.New("task not found")
	}

	urls, err := getUrlsChannel(rc, task, shallow)
	if err != nil {
		return errors.WithStack(err)
	}

	return errors.Wrapf(downloadUrls(rootDir, urls, 4),
		"problem downloading artifacts for task %s", taskId)
}

// searchDependencies does a depth-first search of the dependencies of the "seed" task, returning
// a list of all tasks related to it in the dependency graph. It performs this by doing successive
// calls to the API to crawl the graph, keeping track of any already-processed tasks in the "found"
// map.
func searchDependencies(rc *APIClient, seed *service.RestTask, found map[string]bool) ([]*service.RestTask, error) {
	out := []*service.RestTask{}
	for _, dep := range seed.DependsOn {
		if _, ok := found[dep.TaskId]; ok {
			continue
		}
		t, err := rc.GetTask(dep.TaskId)
		if err != nil {
			return nil, err
		}
		if t != nil {
			found[t.Id] = true
			out = append(out, t)
			more, err := searchDependencies(rc, t, found)
			if err != nil {
				return nil, err
			}
			out = append(out, more...)
			for _, d := range more {
				found[d.Id] = true
			}
		}
	}
	return out, nil
}

type artifactDownload struct {
	url  string
	path string
}

func getArtifactFolderName(task *service.RestTask) string {
	if evergreen.IsPatchRequester(task.Requester) {
		return fmt.Sprintf("artifacts-patch-%v_%v_%v", task.PatchNumber, task.BuildVariant, task.DisplayName)
	}

	if len(task.Revision) >= 5 {
		return fmt.Sprintf("artifacts-%v-%v_%v", task.Revision[0:6], task.BuildVariant, task.DisplayName)
	}
	return fmt.Sprintf("artifacts-%v_%v", task.BuildVariant, task.DisplayName)
}

// getUrlsChannel takes a seed task, and returns a channel that streams all of the artifacts
// associated with the task and its dependencies. If "shallow" is set, only artifacts from the seed
// task will be streamed.
func getUrlsChannel(rc *APIClient, seed *service.RestTask, shallow bool) (chan artifactDownload, error) {
	allTasks := []*service.RestTask{seed}
	if !shallow {
		fmt.Printf("Gathering dependencies... ")
		deps, err := searchDependencies(rc, seed, map[string]bool{})
		if err != nil {
			return nil, err
		}
		allTasks = append(allTasks, deps...)
	}
	fmt.Printf("Done.\n")

	urls := make(chan artifactDownload)
	go func() {
		for _, t := range allTasks {
			for _, f := range t.Files {
				directoryName := getArtifactFolderName(t)
				urls <- artifactDownload{f.URL, directoryName}
			}
		}
		close(urls)
	}()
	return urls, nil
}

func fileNameWithIndex(filename string, index int) string {
	if index-1 == 0 {
		return filename
	}
	parts := strings.Split(filename, ".")
	// If the file has no extension, just append the number with _
	if len(parts) == 1 {
		return fmt.Sprintf("%s_(%d)", filename, index-1)
	}
	// If the file has an extension, add _N (index) just before the extension.
	return fmt.Sprintf("%s_(%d).%s", parts[0], index-1, strings.Join(parts[1:], "."))
}

// downloadUrls pulls a set of artifacts from the given channel and downloads them, using up to
// the given number of workers in parallel. The given root directory determines the base location
// where all the artifact files will be downloaded to.
func downloadUrls(root string, urls chan artifactDownload, workers int) error {
	if workers <= 0 {
		panic("invalid workers count")
	}
	wg := sync.WaitGroup{}
	errs := make(chan error)
	wg.Add(workers)

	// Keep track of filenames being downloaded, so that if there are collisions, we can detect
	// and re-name the file to something else.
	fileNamesUsed := struct {
		nameCounts map[string]int
		sync.Mutex
	}{nameCounts: map[string]int{}}

	for i := 0; i < workers; i++ {
		go func(workerId int) {
			defer wg.Done()
			counter := 0
			for u := range urls {

				// Try to determinate the file location for the output.
				folder := filepath.Join(root, u.path)
				// As a backup plan in case we can't figure out the file name from the URL,
				// the file name will just be named after the worker ID and file index.
				justFile := fmt.Sprintf("%v_%v", workerId, counter)
				parsedUrl, err := url.Parse(u.url)
				if err == nil {
					// under normal operation, the file name written to disk will match the name
					// of the file in the URL. For instance, http://www.website.com/file.tgz
					// will assume "file.tgz".
					pathParts := strings.Split(parsedUrl.Path, "/")
					if len(pathParts) >= 1 {
						justFile = util.CleanForPath(pathParts[len(pathParts)-1])
					}
				}

				fileName := filepath.Join(folder, justFile)
				fileNamesUsed.Lock()
				for {
					fileNamesUsed.nameCounts[fileName] += 1
					testFileName := fileNameWithIndex(fileName, fileNamesUsed.nameCounts[fileName])
					_, err = os.Stat(testFileName)
					if err != nil {
						if os.IsNotExist(err) {
							// we found a file name to safely create without collisions..
							fileName = testFileName
							break
						}
						// something else went wrong.
						errs <- errors.Errorf("failed to check if file exists: %v", err)
						return
					}
				}

				fileNamesUsed.Unlock()

				err = os.MkdirAll(folder, 0777)
				if err != nil {
					errs <- errors.Errorf("Couldn't create output directory %v: %v", folder, err)
					continue
				}

				out, err := os.Create(fileName)
				if err != nil {
					errs <- errors.Errorf("Couldn't download %v: %v", u.url, err)
					continue
				}
				defer out.Close() // nolint
				resp, err := http.Get(u.url)
				if err != nil {
					errs <- errors.Errorf("Couldn't download %v: %v", u.url, err)
					continue
				}
				defer resp.Body.Close() // nolint

				// If we can get the info, determine the file size so that the human can get an
				// idea of how long the file might take to download.
				// TODO: progress bars.
				length, _ := strconv.Atoi(resp.Header.Get("Content-Length"))
				sizeLog := ""
				if length > 0 {
					sizeLog = fmt.Sprintf(" (%s)", humanize.Bytes(uint64(length)))
				}

				justFile = filepath.Base(fileName)
				fmt.Printf("(worker %v) Downloading %v to directory %s%s\n", workerId, justFile, u.path, sizeLog)
				_, err = io.Copy(out, resp.Body)
				if err != nil {
					errs <- errors.Errorf("Couldn't download %v: %v", u.url, err)
					continue
				}
				counter++
			}
		}(i)
	}

	done := make(chan struct{})
	var hasErrors error
	go func() {
		defer close(done)
		for e := range errs {
			hasErrors = errors.New("some files could not be downloaded successfully")
			fmt.Println("error: ", e)
		}
	}()
	wg.Wait()
	close(errs)
	<-done

	return hasErrors
}
