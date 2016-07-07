#!/bin/sh
if [ "Windows_NT" = "$OS" ]
then
    set -o igncr
    export GOBIN=$(cygpath -w `pwd`/bin)
else
    export GOBIN=`pwd`/bin
fi

set -e

# make sure we're in the directory where the script lives
SCRIPT_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})" && pwd)"
cd $SCRIPT_DIR

# remove stale packages
rm -rf vendor/pkg

. ./set_gopath.sh
mkdir -p bin


echo "Building runner..."
go install -ldflags "-X github.com/evergreen-ci/evergreen.BuildRevision=`git rev-parse HEAD`" "./runner/main/runner.go"
echo "Building cli"
go install -ldflags "-X github.com/evergreen-ci/evergreen.BuildRevision=`git rev-parse HEAD`" "./cli/main/cli.go"
echo "Building API server"
go install -ldflags "-X github.com/evergreen-ci/evergreen.BuildRevision=`git rev-parse HEAD`" "./service/api_main/apiserver.go"
echo "Building UI server"
go install -ldflags "-X github.com/evergreen-ci/evergreen.BuildRevision=`git rev-parse HEAD`" "./service/ui_main/ui.go"


# rename API/UI servers and Evergreen runner
echo "Renaming API server..."
mv $GOBIN/apiserver $GOBIN/evergreen_api_server
echo "Renaming UI server..."
mv $GOBIN/ui $GOBIN/evergreen_ui_server
echo "Renaming runner..."
mv $GOBIN/runner $GOBIN/evergreen_runner
