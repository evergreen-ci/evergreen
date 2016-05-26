#!/bin/bash
set -e

# make sure we're in the directory where the script lives
SCRIPT_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})" && pwd)"
cd $SCRIPT_DIR

# set up the $GOPATH appropriately
. ./set_gopath.sh
mkdir -p bin
export GOBIN=bin

GOXC="`pwd`/vendor/src/github.com/laher/goxc/goxc.go"
NONTASKS="go-vet,go-test,archive,rmbin"
DESTDIR="`pwd`/clients"
OSTARGETS=(solaris windows windows darwin darwin linux linux)
ARCHTARGETS=(amd64 amd64 386 amd64 386 amd64 386)

cd cli/main
if [ "$1" = "dev" ] ; then
	# Don't cross compile, just build binaries natively for current platform.

	# If GOOS or GOARCH are not set, assume correct values according to "go env".
	GOOS=${GOOS-`go env GOOS`}
	GOARCH=${GOARCH-`go env GOARCH`}

    mkdir -p $DESTDIR/${GOOS}_${GOARCH};
    go build -o $DESTDIR/${GOOS}_${GOARCH}/main -ldflags "-X github.com/evergreen-ci/evergreen.BuildRevision=`git rev-parse HEAD`" cli.go;
else
	for i in `seq 0 $((${#OSTARGETS[@]}-1))`; do
		export GOOS=${OSTARGETS[i]};
		export GOARCH=${ARCHTARGETS[i]};
		echo "building ${GOOS}_${GOARCH}..."
		mkdir -p $DESTDIR/${GOOS}_${GOARCH};
		go build -o $DESTDIR/${GOOS}_${GOARCH}/main -ldflags "-X github.com/evergreen-ci/evergreen.BuildRevision=`git rev-parse HEAD`" cli.go;
	done
fi
