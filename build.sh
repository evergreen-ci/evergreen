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

for i in apiserver hostinit monitor notify repotracker scheduler taskrunner ui cli; do
	echo "Building ${i}..."
	go install "$i/main/$i.go"
done

# move the api and ui servers to have more explicit names
mv bin/apiserver bin/mci_api_server
mv bin/ui bin/mci_ui_server
