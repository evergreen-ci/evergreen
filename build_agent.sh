#!/bin/sh
set -o errexit

# make sure we're in the directory where the script lives
SCRIPT_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})" && pwd)"
cd $SCRIPT_DIR

# set up the $GOPATH appropriately
. ./set_gopath.sh
mkdir -p bin
export GOBIN=bin

GOXC="`pwd`/vendor/src/github.com/laher/goxc/goxc.go"
NONTASKS="go-vet,go-test,archive,rmbin"
DESTDIR="`pwd`/executables"
OSTARGETS="windows darwin linux solaris"
ARCHTARGETS="amd64 386"

cd agent/main
if [ "$1" = "dev" ] ; then
	# Don't cross compile, just build binaries natively for current platform.
	# If GOOS or GOARCH are not set, assume correct values according to "go env".
	GOOS=${GOOS-`go env GOOS`}
	GOARCH=${GOARCH-`go env GOARCH`}

    mkdir -p $DESTDIR/${GOOS}_${GOARCH};
    go build -o $DESTDIR/${GOOS}_${GOARCH}/main -ldflags "-X github.com/evergreen-ci/evergreen.BuildRevision=`git rev-parse HEAD`" agent.go;
else
	# cross compile.
	go run $GOXC -tasks-=$NONTASKS -d $DESTDIR -os="$OSTARGETS" -arch="$ARCHTARGETS" -build-ldflags "-X github.com/evergreen-ci/evergreen.BuildRevision `git rev-parse HEAD`"
fi

# place the correct built agent githash into the appropriate file
git rev-parse HEAD > "$DESTDIR/version"
