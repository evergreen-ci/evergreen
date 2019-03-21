package bond

import (
	"bufio"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// GetTargetDistro attempts to discover the targeted distro name based
// on the current environment and falls back to the generic builds for
// a platform.
//
// In situations builds target a specific minor release of a platform
// (and specify that version in their name) (e.g. debian and rhel),
// the current platform must match the build
// platform exactly.
func GetTargetDistro() string {
	t, err := getDistro()
	if err != nil {
		grip.Warning(message.WrapError(err, "could not determine distro, falling back to a generic build"))

		if runtime.GOOS == "darwin" {
			return "osx"
		}

		return runtime.GOOS
	}

	return t
}

func getDistro() (string, error) {
	info, err := collectReleaseInfo()
	if err != nil {
		return "", err
	}

	var out string
	if fileExists("/etc/os-release") {
		out = info.getKey("id") + cleanVersion(info.getKey("version_id"))
	} else if fileExists("/etc/lsb-release") {
		out = info.getKey("distro_id") + cleanVersion(info.getKey("distrib_release"))
	} else {
		out = info.getKey("distro_by_induction") + cleanVersion(info.getKey("version_by_induction"))
	}

	if out == "" {
		return "", errors.New("this operating system is not supported, use a generic build")
	}

	return out, nil
}

////////////////////////////////////////////////////////////////////////
//
// utility for collection system information.

// if there was justice in the world, we could just use this, but
// legacy systems are different enough that we can't really trust
// them, and have to do a lot of fiddling.

type releaseInfo struct {
	mu   sync.RWMutex
	data map[string]string
}

func collectReleaseInfo() (*releaseInfo, error) {
	files, err := filepath.Glob("/etc/*release")
	if err != nil {
		return nil, errors.Wrap(err, "problem finding release file")
	}

	if len(files) == 0 {
		return nil, errors.New("found no matching release file")
	}

	info := &releaseInfo{data: map[string]string{}}
	catcher := grip.NewBasicCatcher()
	for _, fn := range files {
		func() {
			file, err := os.Open(fn)
			if err != nil {
				catcher.Add(err)
				return
			}
			defer file.Close()

			scanner := bufio.NewScanner(file)
			contents := []string{}
			for scanner.Scan() {
				line := scanner.Text()
				contents = append(contents, line)
				parts := strings.SplitN(line, "=", 2)
				if len(parts) != 2 {
					continue
				}
				info.data[strings.ToLower(parts[0])] = strings.Trim(parts[1], "\"'")
			}

			catcher.Add(scanner.Err())
		}()
	}

	catcher.Add(info.addExtraInfo())

	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}

	return info, nil
}

func (i *releaseInfo) addExtraInfo() error {
	const (
		versionKey = "version_by_induction"
		distroKey  = "distro_by_induction"
	)

	i.mu.Lock()
	defer i.mu.Unlock()

	if fileExists("/etc/redhat-release") {
		file, err := fileContents("/etc/redhat-release")
		if err != nil {
			return err
		}

		i.data[versionKey] = extractVersion(file)

		if strings.Contains(file, "Red Hat") {
			i.data[distroKey] = "rhel"
		} else if strings.Contains(file, "Fedora") {
			i.data[distroKey] = "fedora"
		}
	} else if fileExists("/etc/system-release") {
		file, err := fileContents("/etc/system-release")
		if err != nil {
			return err
		}

		if strings.Contains(file, "Amazon Linux") {
			i.data[distroKey] = "amzn64"
		}
	} else if commandExists("lsb_release") {
		name, err := commandOutput("lsb_release", "-s", "-i")
		if err != nil {
			return err
		}

		if name != "RedHatEnterpriseServer" || !strings.Contains(name, "CentOS") {
			return errors.Errorf("release named %s is not supported", name)
		}

		ver, err := commandOutput("lsb_release", "-s", "-r")
		if err != nil {
			return err
		}

		i.data[versionKey] = ver
		i.data[distroKey] = "rhel"
	}

	return nil
}

func (i *releaseInfo) getKey(key string) string {
	i.mu.RLock()
	defer i.mu.RUnlock()

	return i.data[key]
}

////////////////////////////////////////////////////////////////////////
//
// util functions

func fileExists(path string) bool {
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return true
	}

	return false
}

func fileContents(path string) (string, error) {
	out, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func commandExists(cmd string) bool {
	cmd, err := exec.LookPath(cmd)
	if err != nil || cmd == "" {
		return false
	}

	return true
}

func commandOutput(cmd string, args ...string) (string, error) {
	out, err := exec.Command(cmd, args...).CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func cleanVersion(s string) string { return strings.Replace(s, ".", "", -1) }

func extractVersion(s string) string {
	re, err := regexp.Compile(`.*(\d*\.*\d*).*`)
	if err != nil {
		return ""
	}
	parts := re.FindAllString(s, 1)
	if len(parts) != 1 {
		return ""
	}

	return parts[0]
}
