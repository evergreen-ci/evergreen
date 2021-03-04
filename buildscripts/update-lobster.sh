#!/bin/bash

# this script pulls the evergreen-ci/lobster repo and copies the files so that
# they can be served by evergreen
# it should only be run from the base evergreen directory
set -o errexit
set -o xtrace

SCRIPTS_DIR=buildscripts
LOBSTER_REPO=https://github.com/evergreen-ci/lobster.git
LOBSTER_DIR=.lobster-temp
LOBSTER_ASSETS_DIR=build
LOBSTER_STATIC_DIR=static
LOBSTER_HTML=index.html
EVG_LOB_STATIC_DIR=public/static/lobster/static
EVG_LOB_MANIFEST_DIR=public/static/lobster
EVERGREEN_JS_DIR=js
EVERGREEN_CSS_DIR=css
EVERGREEN_TEMPLATES_DIR=service/templates

# clone lobster repo
pushd $SCRIPTS_DIR
rm -rf $LOBSTER_DIR
git clone $LOBSTER_REPO $LOBSTER_DIR

# build lobster
pushd $LOBSTER_DIR
npm install
# env variables used during lobster build step
export PUBLIC_URL=/static/lobster
export REACT_APP_LOGKEEPER_BASE=https://logkeeper.mongodb.org
export REACT_APP_EVERGREEN_BASE=https://evergreen.mongodb.com
#
npm run build 
popd
popd # cwd should be evergreen repo base now

LOBSTER_DIR_FROM_BASE="$SCRIPTS_DIR/$LOBSTER_DIR"

# delete lobster static files in evergreen repo
rm -rf $EVG_LOB_STATIC_DIR
mkdir -p $EVG_LOB_STATIC_DIR

# copy files from lobster to evergreen
cp -R $LOBSTER_DIR_FROM_BASE/$LOBSTER_ASSETS_DIR/$LOBSTER_STATIC_DIR/ $EVG_LOB_STATIC_DIR/
cp $LOBSTER_DIR_FROM_BASE/$LOBSTER_ASSETS_DIR/$LOBSTER_HTML $EVERGREEN_TEMPLATES_DIR/temp.html
cp $LOBSTER_DIR_FROM_BASE/$LOBSTER_ASSETS_DIR/manifest.json $EVG_LOB_MANIFEST_DIR
# surround the html with go template tags
pushd $EVERGREEN_TEMPLATES_DIR
echo pwd
echo -e "{{define \"base\"}}\n$(cat temp.html)" > lobster.html
echo "{{end}}" >> lobster.html
rm temp.html
popd
rm -rf $LOBSTER_DIR_FROM_BASE
echo "finished updating lobster"
