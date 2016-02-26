#!/bin/sh
if [ "Windows_NT" = "$OS" ]
then
    set -o igncr
fi

set -e

# make sure we're in the directory where the script lives
SCRIPT_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})" && pwd)"
cd $SCRIPT_DIR

# remove stale packages
rm -rf vendor/pkg

. ./set_gopath.sh
mkdir -p bin
export GOBIN=`pwd`/bin

for i in apiserver ui runner cli; do
  echo "Building ${i}..."
  go install $1 -ldflags "-X github.com/evergreen-ci/evergreen.BuildRevision=`git rev-parse HEAD`" "$i/main/$i.go"
done

# rename API/UI servers and Evergreen runner
echo "Renaming API server..."
mv bin/apiserver bin/evergreen_api_server
echo "Renaming UI server..."
mv bin/ui bin/evergreen_ui_server
echo "Renaming runner..."
mv bin/runner bin/evergreen_runner
