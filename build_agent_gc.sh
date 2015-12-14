#!/bin/sh
set -o errexit

# make sure we're in the directory where the script lives
SCRIPT_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})" && pwd)"
cd $SCRIPT_DIR

# set up the $GOPATH appropriately
. ./set_gopath.sh
mkdir -p bin
export GOBIN=bin
DESTDIR="`pwd`/executables/snapshot"

OSTARGETS=(  windows windows darwin darwin linux linux solaris)
ARCHTARGETS=(amd64   386     amd64  386    amd64 386   amd64)

for i in `seq 0 $((${#OSTARGETS[@]}-1))`; do
    export GOOS=${OSTARGETS[i]};
    export GOARCH=${ARCHTARGETS[i]};
    echo "building ${GOOS}_${GOARCH}..."
    mkdir -p $DESTDIR/${GOOS}_${GOARCH};
    go build -o $DESTDIR/${GOOS}_${GOARCH}/main -ldflags "-X github.com/evergreen-ci/evergreen.BuildRevision=`git rev-parse HEAD`" agent/main/agent.go;
done

# place the correct built agent githash into the appropriate file
git rev-parse HEAD > "$DESTDIR/version"
