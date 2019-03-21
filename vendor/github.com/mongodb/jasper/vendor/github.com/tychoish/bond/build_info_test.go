package bond

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTargetIdentification(t *testing.T) {
	assert := assert.New(t)

	builds := map[string]string{
		"https://downloads.mongodb.com/linux/mongodb-linux-arm64-enterprise-ubuntu1604-3.4.0-rc5.tgz":  "ubuntu1604",
		"https://downloads.mongodb.com/linux/mongodb-linux-ppc64le-enterprise-rhel71-3.4.0-rc5.tgz":    "rhel71",
		"https://downloads.mongodb.com/linux/mongodb-linux-s390x-enterprise-rhel72-3.4.0-rc5.tgz":      "rhel72",
		"https://downloads.mongodb.com/linux/mongodb-linux-x86_64-enterprise-amzn64-3.4.0-rc5.tgz":     "amzn64",
		"https://downloads.mongodb.com/linux/mongodb-linux-x86_64-enterprise-debian71-3.4.0-rc5.tgz":   "debian71",
		"https://downloads.mongodb.com/linux/mongodb-linux-x86_64-enterprise-debian81-3.4.0-rc5.tgz":   "debian81",
		"https://downloads.mongodb.com/linux/mongodb-linux-x86_64-enterprise-rhel62-3.4.0-rc5.tgz":     "rhel62",
		"https://downloads.mongodb.com/linux/mongodb-linux-x86_64-enterprise-rhel70-3.4.0-rc5.tgz":     "rhel70",
		"https://downloads.mongodb.com/linux/mongodb-linux-x86_64-enterprise-suse11-3.4.0-rc5.tgz":     "suse11",
		"https://downloads.mongodb.com/linux/mongodb-linux-x86_64-enterprise-suse12-3.4.0-rc5.tgz":     "suse12",
		"https://downloads.mongodb.com/linux/mongodb-linux-x86_64-enterprise-ubuntu1204-3.4.0-rc5.tgz": "ubuntu1204",
		"https://downloads.mongodb.com/linux/mongodb-linux-x86_64-enterprise-ubuntu1404-3.4.0-rc5.tgz": "ubuntu1404",
		"https://downloads.mongodb.com/linux/mongodb-linux-x86_64-enterprise-ubuntu1604-3.4.0-rc5.tgz": "ubuntu1604",
		"https://downloads.mongodb.com/osx/mongodb-osx-x86_64-enterprise-3.4.0-rc5.tgz":                "osx",
		"https://downloads.mongodb.com/win32/mongodb-win32-x86_64-enterprise-windows-64-3.4.0-rc5.zip": "windows",
		"https://fastdl.mongodb.org/linux/mongodb-linux-arm64-ubuntu1604-3.4.0-rc5.tgz":                "ubuntu1604",
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-3.4.0-rc5.tgz":                          "linux",
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-amazon-3.4.0-rc5.tgz":                   "amazon",
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-debian71-3.4.0-rc5.tgz":                 "debian71",
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-debian81-3.4.0-rc5.tgz":                 "debian81",
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-rhel62-3.4.0-rc5.tgz":                   "rhel62",
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-rhel70-3.4.0-rc5.tgz":                   "rhel70",
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-suse11-3.4.0-rc5.tgz":                   "suse11",
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-suse12-3.4.0-rc5.tgz":                   "suse12",
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-ubuntu1204-3.4.0-rc5.tgz":               "ubuntu1204",
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-ubuntu1404-3.4.0-rc5.tgz":               "ubuntu1404",
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-ubuntu1604-3.4.0-rc5.tgz":               "ubuntu1604",
		"https://fastdl.mongodb.org/osx/mongodb-osx-ssl-x86_64-3.4.0-rc5.tgz":                          "osx-ssl",
		"https://fastdl.mongodb.org/osx/mongodb-osx-x86_64-3.4.0-rc5.tgz":                              "osx",
		"https://fastdl.mongodb.org/sunos5/mongodb-sunos5-x86_64-3.4.0-rc5.tgz":                        "sunos5",
		"https://fastdl.mongodb.org/win32/mongodb-win32-x86_64-2008plus-3.4.0-rc5.zip":                 "windows_x86_64-2008plus",
		"https://fastdl.mongodb.org/win32/mongodb-win32-x86_64-2008plus-ssl-3.4.0-rc5.zip":             "windows_x86_64-2008plus-ssl",
		"https://fastdl.mongodb.org/win32/mongodb-win32-x86_64-3.4.0-rc5.zip":                          "windows_x86_64",

		// targets removed before 3.4.0 that older versions still support
		"https://fastdl.mongodb.org/win32/mongodb-win32-i386-2.6.9.zip": "windows_i686",
		"https://fastdl.mongodb.org/linux/mongodb-linux-i386-2.6.9.tgz": "linux",
	}

	for url, target := range builds {
		fn := filepath.Base(url)
		fn = fn[:len(fn)-4]

		out, err := getTarget(fn)
		assert.NoError(err)
		assert.Equal(target, out, fmt.Sprintln(target, "-->", url))
	}

	out, err := getTarget("invalid")
	assert.Error(err)
	assert.Equal("", out)
}

func TestEditionIdentification(t *testing.T) {
	assert := assert.New(t)

	builds := map[string]MongoDBEdition{
		"https://downloads.mongodb.com/linux/mongodb-linux-arm64-enterprise-ubuntu1604-3.4.0-rc5.tgz":   Enterprise,
		"https://downloads.mongodb.com/linux/mongodb-linux-ppc64le-enterprise-rhel71-3.4.0-rc5.tgz":     Enterprise,
		"https://downloads.mongodb.com/linux/mongodb-linux-s390x-enterprise-rhel72-3.4.0-rc5.tgz":       Enterprise,
		"https://downloads.mongodb.com/linux/mongodb-linux-x86_64-enterprise-amzn64-3.4.0-rc5.tgz":      Enterprise,
		"https://downloads.mongodb.com/linux/mongodb-linux-x86_64-enterprise-debian71-3.4.0-rc5.tgz":    Enterprise,
		"https://downloads.mongodb.com/linux/mongodb-linux-x86_64-enterprise-debian81-3.4.0-rc5.tgz":    Enterprise,
		"https://downloads.mongodb.com/linux/mongodb-linux-x86_64-enterprise-rhel62-3.4.0-rc5.tgz":      Enterprise,
		"https://downloads.mongodb.com/linux/mongodb-linux-x86_64-enterprise-rhel70-3.4.0-rc5.tgz":      Enterprise,
		"https://downloads.mongodb.com/linux/mongodb-linux-x86_64-enterprise-suse11-3.4.0-rc5.tgz":      Enterprise,
		"https://downloads.mongodb.com/linux/mongodb-linux-x86_64-enterprise-suse12-3.4.0-rc5.tgz":      Enterprise,
		"https://downloads.mongodb.com/linux/mongodb-linux-x86_64-enterprise-ubuntu1204-3.4.0-rc5.tgz":  Enterprise,
		"https://downloads.mongodb.com/linux/mongodb-linux-x86_64-enterprise-ubuntu1404-3.4.0-rc5.tgz":  Enterprise,
		"https://downloads.mongodb.com/linux/mongodb-linux-x86_64-enterprise-ubuntu1604-3.4.0-rc5. tgz": Enterprise,
		"https://downloads.mongodb.com/osx/mongodb-osx-x86_64-enterprise-3.4.0-rc5.tgz":                 Enterprise,
		"https://downloads.mongodb.com/win32/mongodb-win32-x86_64-enterprise-windows-64-3.4.0-rc5.zip":  Enterprise,
		"https://fastdl.mongodb.org/linux/mongodb-linux-arm64-ubuntu1604-3.4.0-rc5.tgz":                 CommunityTargeted,
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-amazon-3.4.0-rc5.tgz":                    CommunityTargeted,
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-debian71-3.4.0-rc5.tgz":                  CommunityTargeted,
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-debian81-3.4.0-rc5.tgz":                  CommunityTargeted,
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-rhel62-3.4.0-rc5.tgz":                    CommunityTargeted,
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-rhel70-3.4.0-rc5.tgz":                    CommunityTargeted,
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-suse11-3.4.0-rc5.tgz":                    CommunityTargeted,
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-suse12-3.4.0-rc5.tgz":                    CommunityTargeted,
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-ubuntu1204-3.4.0-rc5.tgz":                CommunityTargeted,
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-ubuntu1404-3.4.0-rc5.tgz":                CommunityTargeted,
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-ubuntu1604-3.4.0-rc5.tgz":                CommunityTargeted,
		"https://fastdl.mongodb.org/osx/mongodb-osx-ssl-x86_64-3.4.0-rc5.tgz":                           CommunityTargeted,
		"https://fastdl.mongodb.org/osx/mongodb-osx-x86_64-3.4.0-rc5.tgz":                               Base,
		"https://fastdl.mongodb.org/sunos5/mongodb-sunos5-x86_64-3.4.0-rc5.tgz":                         Base,
		"https://fastdl.mongodb.org/win32/mongodb-win32-x86_64-2008plus-3.4.0-rc5.zip":                  Base,
		"https://fastdl.mongodb.org/win32/mongodb-win32-x86_64-2008plus-ssl-3.4.0-rc5.zip":              Base,
		"https://fastdl.mongodb.org/win32/mongodb-win32-x86_64-3.4.0-rc5.zip":                           Base,
		"https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-3.4.0-rc5.tgz":                           Base,
	}

	for url, edition := range builds {
		out, err := getEdition(filepath.Base(url))
		assert.NoError(err)
		assert.Equal(edition, out)
	}

	out, err := getEdition("invalid")
	assert.Error(err)
	assert.Equal(MongoDBEdition(""), out)
}

func TestVersionIdentification(t *testing.T) {
	assert := assert.New(t)

	builds := map[string]string{
		"mongodb-linux-x86_64-3.4.0-rc5":  "3.4.0-rc5",
		"mongodb-linux-x86_64-3.4.0-rc4":  "3.4.0-rc4",
		"mongodb-linux-x86_64-3.4.0-rc3":  "3.4.0-rc3",
		"mongodb-linux-x86_64-3.4.0-rc2":  "3.4.0-rc2",
		"mongodb-linux-x86_64-3.4.0-rc1":  "3.4.0-rc1",
		"mongodb-linux-x86_64-3.4.0-rc0":  "3.4.0-rc0",
		"mongodb-linux-x86_64-3.3.15":     "3.3.15",
		"mongodb-linux-x86_64-3.3.14":     "3.3.14",
		"mongodb-linux-x86_64-3.3.13":     "3.3.13",
		"mongodb-linux-x86_64-3.3.12":     "3.3.12",
		"mongodb-linux-x86_64-3.3.11":     "3.3.11",
		"mongodb-linux-x86_64-3.3.10":     "3.3.10",
		"mongodb-linux-x86_64-3.3.9":      "3.3.9",
		"mongodb-linux-x86_64-3.3.8":      "3.3.8",
		"mongodb-linux-x86_64-3.3.7":      "3.3.7",
		"mongodb-linux-x86_64-3.3.6":      "3.3.6",
		"mongodb-linux-x86_64-3.3.5":      "3.3.5",
		"mongodb-linux-x86_64-3.3.4":      "3.3.4",
		"mongodb-linux-x86_64-3.3.3":      "3.3.3",
		"mongodb-linux-x86_64-3.3.2":      "3.3.2",
		"mongodb-linux-x86_64-3.3.1":      "3.3.1",
		"mongodb-linux-x86_64-3.3.0":      "3.3.0",
		"mongodb-linux-x86_64-3.2.11":     "3.2.11",
		"mongodb-linux-x86_64-3.2.11-rc1": "3.2.11-rc1",
		"mongodb-linux-x86_64-3.2.11-rc0": "3.2.11-rc0",
		"mongodb-linux-x86_64-3.2.10":     "3.2.10",
		"mongodb-linux-x86_64-3.2.10-rc2": "3.2.10-rc2",
		"mongodb-linux-x86_64-3.2.10-rc1": "3.2.10-rc1",
		"mongodb-linux-x86_64-3.2.10-rc0": "3.2.10-rc0",
		"mongodb-linux-x86_64-3.2.9":      "3.2.9",
		"mongodb-linux-x86_64-3.2.9-rc1":  "3.2.9-rc1",
		"mongodb-linux-x86_64-3.2.8":      "3.2.8",
		"mongodb-linux-x86_64-3.2.8-rc1":  "3.2.8-rc1",
		"mongodb-linux-x86_64-3.2.8-rc0":  "3.2.8-rc0",
		"mongodb-linux-x86_64-3.2.7":      "3.2.7",
		"mongodb-linux-x86_64-3.2.7-rc1":  "3.2.7-rc1",
		"mongodb-linux-x86_64-3.2.6":      "3.2.6",
		"mongodb-linux-x86_64-3.2.6-rc0":  "3.2.6-rc0",
		"mongodb-linux-x86_64-3.2.5":      "3.2.5",
		"mongodb-linux-x86_64-3.2.5-rc1":  "3.2.5-rc1",
		"mongodb-linux-x86_64-3.2.5-rc0":  "3.2.5-rc0",
		"mongodb-linux-x86_64-3.2.4":      "3.2.4",
		"mongodb-linux-x86_64-3.2.4-rc0":  "3.2.4-rc0",
		"mongodb-linux-x86_64-3.2.3":      "3.2.3",
		"mongodb-linux-x86_64-3.2.2":      "3.2.2",
		"mongodb-linux-x86_64-3.2.2-rc2":  "3.2.2-rc2",
		"mongodb-linux-x86_64-3.2.2-rc1":  "3.2.2-rc1",
		"mongodb-linux-x86_64-3.2.2-rc0":  "3.2.2-rc0",
		"mongodb-linux-x86_64-3.2.1":      "3.2.1",
		"mongodb-linux-x86_64-3.2.1-rc3":  "3.2.1-rc3",
		"mongodb-linux-x86_64-3.2.1-rc2":  "3.2.1-rc2",
		"mongodb-linux-x86_64-3.2.1-rc1":  "3.2.1-rc1",
		"mongodb-linux-x86_64-3.2.1-rc0":  "3.2.1-rc0",
		"mongodb-linux-x86_64-3.2.0":      "3.2.0",
		"mongodb-linux-x86_64-3.2.0-rc6":  "3.2.0-rc6",
		"mongodb-linux-x86_64-3.2.0-rc5":  "3.2.0-rc5",
		"mongodb-linux-x86_64-3.2.0-rc4":  "3.2.0-rc4",
		"mongodb-linux-x86_64-3.2.0-rc3":  "3.2.0-rc3",
		"mongodb-linux-x86_64-3.2.0-rc2":  "3.2.0-rc2",
		"mongodb-linux-x86_64-3.2.0-rc1":  "3.2.0-rc1",
		"mongodb-linux-x86_64-3.2.0-rc0":  "3.2.0-rc0",
		"mongodb-linux-x86_64-3.1.9":      "3.1.9",
		"mongodb-linux-x86_64-3.1.8":      "3.1.8",
		"mongodb-linux-x86_64-3.1.7":      "3.1.7",
		"mongodb-linux-x86_64-3.1.6":      "3.1.6",
		"mongodb-linux-x86_64-3.1.5":      "3.1.5",
		"mongodb-linux-x86_64-3.1.4":      "3.1.4",
		"mongodb-linux-x86_64-3.1.3":      "3.1.3",
		"mongodb-linux-x86_64-3.1.2":      "3.1.2",
		"mongodb-linux-x86_64-3.1.1":      "3.1.1",
		"mongodb-linux-x86_64-3.1.0":      "3.1.0",
		"mongodb-linux-x86_64-3.0.14":     "3.0.14",
		"mongodb-linux-x86_64-3.0.13":     "3.0.13",
		"mongodb-linux-x86_64-3.0.13-rc0": "3.0.13-rc0",
		"mongodb-linux-x86_64-3.0.12":     "3.0.12",
		"mongodb-linux-x86_64-3.0.12-rc0": "3.0.12-rc0",
		"mongodb-linux-x86_64-3.0.11":     "3.0.11",
		"mongodb-linux-x86_64-3.0.10":     "3.0.10",
		"mongodb-linux-x86_64-3.0.10-rc1": "3.0.10-rc1",
		"mongodb-linux-x86_64-3.0.10-rc0": "3.0.10-rc0",
		"mongodb-linux-x86_64-3.0.9":      "3.0.9",
		"mongodb-linux-x86_64-3.0.9-rc0":  "3.0.9-rc0",
		"mongodb-linux-x86_64-3.0.8":      "3.0.8",
		"mongodb-linux-x86_64-3.0.8-rc0":  "3.0.8-rc0",
		"mongodb-linux-x86_64-3.0.7":      "3.0.7",
		"mongodb-linux-x86_64-3.0.7-rc0":  "3.0.7-rc0",
		"mongodb-linux-x86_64-3.0.6":      "3.0.6",
		"mongodb-linux-x86_64-3.0.6-rc2":  "3.0.6-rc2",
		"mongodb-linux-x86_64-3.0.6-rc0":  "3.0.6-rc0",
		"mongodb-linux-x86_64-3.0.5":      "3.0.5",
		"mongodb-linux-x86_64-3.0.5-rc2":  "3.0.5-rc2",
		"mongodb-linux-x86_64-3.0.5-rc1":  "3.0.5-rc1",
		"mongodb-linux-x86_64-3.0.5-rc0":  "3.0.5-rc0",
		"mongodb-linux-x86_64-3.0.4":      "3.0.4",
		"mongodb-linux-x86_64-3.0.4-rc0":  "3.0.4-rc0",
		"mongodb-linux-x86_64-3.0.3":      "3.0.3",
		"mongodb-linux-x86_64-3.0.3-rc2":  "3.0.3-rc2",
		"mongodb-linux-x86_64-3.0.3-rc1":  "3.0.3-rc1",
		"mongodb-linux-x86_64-3.0.3-rc0":  "3.0.3-rc0",
		"mongodb-linux-x86_64-3.0.2":      "3.0.2",
		"mongodb-linux-x86_64-3.0.2-rc0":  "3.0.2-rc0",
		"mongodb-linux-x86_64-3.0.1":      "3.0.1",
		"mongodb-linux-x86_64-3.0.1-rc0":  "3.0.1-rc0",
		"mongodb-linux-x86_64-3.0.0":      "3.0.0",
		"mongodb-linux-x86_64-3.0.0-rc11": "3.0.0-rc11",
		"mongodb-linux-x86_64-3.0.0-rc10": "3.0.0-rc10",
		"mongodb-linux-x86_64-3.0.0-rc9":  "3.0.0-rc9",
		"mongodb-linux-x86_64-3.0.0-rc8":  "3.0.0-rc8",
		"mongodb-linux-x86_64-3.0.0-rc7":  "3.0.0-rc7",
		"mongodb-linux-x86_64-3.0.0-rc6":  "3.0.0-rc6",
		"mongodb-linux-x86_64-2.8.0-rc5":  "2.8.0-rc5",
		"mongodb-linux-x86_64-2.8.0-rc4":  "2.8.0-rc4",
		"mongodb-linux-x86_64-2.8.0-rc3":  "2.8.0-rc3",
		"mongodb-linux-x86_64-2.8.0-rc2":  "2.8.0-rc2",
		"mongodb-linux-x86_64-2.8.0-rc1":  "2.8.0-rc1",
		"mongodb-linux-x86_64-2.8.0-rc0":  "2.8.0-rc0",
		"mongodb-linux-x86_64-2.7.8":      "2.7.8",
		"mongodb-linux-x86_64-2.7.7":      "2.7.7",
		"mongodb-linux-x86_64-2.7.6":      "2.7.6",
		"mongodb-linux-x86_64-2.7.5":      "2.7.5",
		"mongodb-linux-x86_64-2.7.4":      "2.7.4",
		"mongodb-linux-x86_64-2.7.3":      "2.7.3",
		"mongodb-linux-x86_64-2.7.2":      "2.7.2",
		"mongodb-linux-x86_64-2.7.1":      "2.7.1",
		"mongodb-linux-x86_64-2.7.0":      "2.7.0",
		"mongodb-linux-x86_64-2.6.12":     "2.6.12",
		"mongodb-linux-x86_64-2.6.12-rc0": "2.6.12-rc0",
		"mongodb-linux-x86_64-2.6.11":     "2.6.11",
		"mongodb-linux-x86_64-2.6.11-rc0": "2.6.11-rc0",
		"mongodb-linux-x86_64-2.6.10":     "2.6.10",
		"mongodb-linux-x86_64-2.6.10-rc0": "2.6.10-rc0",
		"mongodb-linux-x86_64-2.6.9":      "2.6.9",
		"mongodb-linux-x86_64-2.6.9-rc0":  "2.6.9-rc0",
		"mongodb-linux-x86_64-2.6.8":      "2.6.8",
		"mongodb-linux-x86_64-2.6.8-rc0":  "2.6.8-rc0",
		"mongodb-linux-x86_64-2.6.7":      "2.6.7",
		"mongodb-linux-x86_64-2.6.7-rc0":  "2.6.7-rc0",
		"mongodb-linux-x86_64-2.6.6":      "2.6.6",
		"mongodb-linux-x86_64-2.6.6-rc0":  "2.6.6-rc0",
		"mongodb-linux-x86_64-2.6.5":      "2.6.5",
		"mongodb-linux-x86_64-2.6.5-rc4":  "2.6.5-rc4",
		"mongodb-linux-x86_64-2.6.5-rc3":  "2.6.5-rc3",
		"mongodb-linux-x86_64-2.6.5-rc2":  "2.6.5-rc2",
		"mongodb-linux-x86_64-2.6.4":      "2.6.4",
		"mongodb-linux-x86_64-2.6.4-rc1":  "2.6.4-rc1",
		"mongodb-linux-x86_64-2.6.3":      "2.6.3",
		"mongodb-linux-x86_64-2.6.2":      "2.6.2",
		"mongodb-linux-x86_64-2.6.2-rc1":  "2.6.2-rc1",
		"mongodb-linux-x86_64-2.6.2-rc0":  "2.6.2-rc0",
		"mongodb-linux-x86_64-2.6.1":      "2.6.1",
		"mongodb-linux-x86_64-2.6.1-rc1":  "2.6.1-rc1",
		"mongodb-linux-x86_64-2.6.1-rc0":  "2.6.1-rc0",
		"mongodb-linux-x86_64-2.6.0":      "2.6.0",
		"mongodb-linux-x86_64-2.6.0-rc3":  "2.6.0-rc3",
		"mongodb-linux-x86_64-2.6.0-rc2":  "2.6.0-rc2",
		"mongodb-linux-x86_64-2.6.0-rc1":  "2.6.0-rc1",
		"mongodb-linux-x86_64-2.6.0-rc0":  "2.6.0-rc0",
		"mongodb-linux-x86_64-2.5.5":      "2.5.5",
		"mongodb-linux-x86_64-2.5.4":      "2.5.4",
		"mongodb-linux-x86_64-2.5.3":      "2.5.3",
		"mongodb-linux-x86_64-2.5.2":      "2.5.2",
		"mongodb-linux-x86_64-2.5.1":      "2.5.1",
		"mongodb-linux-x86_64-2.5.0":      "2.5.0",
		"mongodb-linux-x86_64-2.4.14":     "2.4.14",
		"mongodb-linux-x86_64-2.4.14-rc0": "2.4.14-rc0",
		"mongodb-linux-x86_64-2.4.13":     "2.4.13",
		"mongodb-linux-x86_64-2.4.13-rc0": "2.4.13-rc0",
		"mongodb-linux-x86_64-2.4.12":     "2.4.12",
		"mongodb-linux-x86_64-2.4.12-rc0": "2.4.12-rc0",
		"mongodb-linux-x86_64-2.4.11":     "2.4.11",
		"mongodb-linux-x86_64-2.4.11-rc0": "2.4.11-rc0",
		"mongodb-linux-x86_64-2.4.10":     "2.4.10",
		"mongodb-linux-x86_64-2.4.10-rc0": "2.4.10-rc0",
		"mongodb-linux-x86_64-2.4.9":      "2.4.9",
		"mongodb-linux-x86_64-2.4.9-rc0":  "2.4.9-rc0",
		"mongodb-linux-x86_64-2.4.8":      "2.4.8",
		"mongodb-linux-x86_64-2.4.7":      "2.4.7",
		"mongodb-linux-x86_64-2.4.7-rc0":  "2.4.7-rc0",
		"mongodb-linux-x86_64-2.4.6":      "2.4.6",
		"mongodb-linux-x86_64-2.4.6-rc1":  "2.4.6-rc1",
		"mongodb-linux-x86_64-2.4.6-rc0":  "2.4.6-rc0",
		"mongodb-linux-x86_64-2.4.5":      "2.4.5",
		"mongodb-linux-x86_64-2.4.5-rc0":  "2.4.5-rc0",
		"mongodb-linux-x86_64-2.4.4":      "2.4.4",
		"mongodb-linux-x86_64-2.4.4-rc0":  "2.4.4-rc0",
		"mongodb-linux-x86_64-2.4.3":      "2.4.3",
		"mongodb-linux-x86_64-2.4.3-rc0":  "2.4.3-rc0",
		"mongodb-linux-x86_64-2.4.2":      "2.4.2",
		"mongodb-linux-x86_64-2.4.2-rc0":  "2.4.2-rc0",
		"mongodb-linux-x86_64-2.4.1":      "2.4.1",
		"mongodb-linux-x86_64-2.4.0":      "2.4.0",
		"mongodb-linux-x86_64-2.4.0-rc3":  "2.4.0-rc3",
		"mongodb-linux-x86_64-2.4.0-rc2":  "2.4.0-rc2",
		"mongodb-linux-x86_64-2.4.0-rc1":  "2.4.0-rc1",
		"mongodb-linux-x86_64-2.4.0-rc0":  "2.4.0-rc0",

		// latest
		"mongodb-linux-x86_64-v2.4-latest": "2.4-latest",
		"mongodb-linux-x86_64-v2.6-latest": "2.6-latest",
		"mongodb-linux-x86_64-v3.2-latest": "3.2-latest",
		"mongodb-linux-x86_64-v3.0-latest": "3.0-latest",
		"mongodb-linux-x86_64-latest":      "latest",
	}

	for dir, version := range builds {
		v, err := getVersion(dir, Base)
		assert.Equal(version, v)
		assert.NoError(err)
	}

	// various error cases
	for _, fn := range []string{"invalid", "invalid-invalid", "invalid-invalid-invalid"} {
		v, err := getVersion(fn, Base)
		assert.Equal("", v)
		assert.Error(err)
	}
}
