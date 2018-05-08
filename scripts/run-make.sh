#!/bin/bash

set -o xtrace
set -o errexit

# Configure a local go path for this build.
export GOPATH=$1/gopath

# Set the path to nodejs binaries
export PATH=/opt/node/bin:gopath/src/github.com/evergreen-ci/evergreen/public/node_modules/.bin:/opt/go1.8/go/bin:$PATH

# configure path to the settings file
export SETTINGS_OVERRIDE=creds.yml

# on windows we need to turn the slashes the other way
if [ "Windows_NT" == "$OS" ]; then
   export GOPATH=$(cygpath -m $GOPATH)
   export SETTINGS_OVERRIDE=$(cygpath -m $SETTINGS_OVERRIDE)
fi

# Run make, called with proper environment variables set,
# running the target.
echo $2
$2 make $3 $4
