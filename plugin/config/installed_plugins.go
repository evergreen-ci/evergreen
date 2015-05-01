// The plugin/config package is used to manage which plugins
// are imported into MCI. The installed_plugins.go file contains
// an import for each plugin package we would like to use.
//
// Plugins publish themselves to the mci/plugin package on program initialization.
// This means that once we've  written a package that properly registers a plugin,
// all we have to do to make the mci/plugin package aware of it is to import
// the new package somewhere (i.e. in installed_plugins.go).
//
// To install a new plugin, simply get it:
//  go get host.com/path/to/new/plugin
// And import it in installed_plugins.go with:
//  import _ "host.com/path/to/new/plugin"
// (the underscore after the import is used to tell Go we just want to import
// the package for its initialization function)
package config

// ===== PLUGINS INCLUDED WITH MCI =====
import _ "github.com/evergreen-ci/evergreen/plugin/builtin/archive"
import _ "github.com/evergreen-ci/evergreen/plugin/builtin/attach"
import _ "github.com/evergreen-ci/evergreen/plugin/builtin/expansions"
import _ "github.com/evergreen-ci/evergreen/plugin/builtin/git"
import _ "github.com/evergreen-ci/evergreen/plugin/builtin/helloworld"
import _ "github.com/evergreen-ci/evergreen/plugin/builtin/gotest"
import _ "github.com/evergreen-ci/evergreen/plugin/builtin/s3Plugin"
import _ "github.com/evergreen-ci/evergreen/plugin/builtin/s3copy"
import _ "github.com/evergreen-ci/evergreen/plugin/builtin/shell"
