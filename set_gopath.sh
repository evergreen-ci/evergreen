#!/bin/bash

MCI_PKG='github.com/evergreen-ci/evergreen'

setgopath() {
    if [ "Windows_NT" != "$OS" ]; then
        SOURCE_GOPATH=`pwd`.gopath
        VENDOR_GOPATH=`pwd`/vendor

        # set up the $GOPATH to use the vendored dependencies as
        # well as the source 
        rm -rf .gopath/
        mkdir -p .gopath/src/"$(dirname "${MCI_PKG}")"
        ln -sf `pwd` .gopath/src/$MCI_PKG
        export GOPATH=`pwd`/vendor:`pwd`/.gopath

    else
        echo "using windows + cygwin gopath [COPY ONLY]"
        local SOURCE_GOPATH=`pwd`/.gopath
        local VENDOR_GOPATH=`pwd`/vendor
        SOURCE_GOPATH=$(cygpath -w $SOURCE_GOPATH);
        VENDOR_GOPATH=$(cygpath -w $VENDOR_GOPATH);

        # set up the $GOPATH to use the vendored dependencies as
        # well as the source for the mongo tools
        rm -rf .gopath/
        mkdir -p .gopath/src/"$MCI_PKG"
        cp -r `pwd`/* .gopath/src/$MCI_PKG
        # now handle vendoring
        rm -rf .gopath/src/$MCI_PKG/vendor 
        cp -r `pwd`/vendor/src/* .gopath/src/.
        export GOPATH="$SOURCE_GOPATH;$VENDOR_GOPATH"
    fi;
}

setgopath
