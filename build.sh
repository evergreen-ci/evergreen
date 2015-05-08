#!/bin/sh
set -o errexit

# make sure we're in the directory where the script lives
SCRIPT_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})" && pwd)"
cd $SCRIPT_DIR

# remove stale packages
rm -rf vendor/pkg

. ./set_gopath.sh
mkdir -p bin
export GOBIN=bin

for i in apiserver ui runner cli hostinit monitor notify repotracker scheduler taskrunner; do
	echo "Building ${i}..."
	go install "$i/main/$i.go"
done

# rename API/UI servers and Evergreen runner
echo "Renaming API server..."
mv bin/apiserver bin/evergreen_api_server
echo "Renaming UI server..."
mv bin/ui bin/evergreen_ui_server
echo "Renaming runner..."
mv bin/runner bin/evergreen_runner
