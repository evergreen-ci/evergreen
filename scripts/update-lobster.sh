#!/bin/bash

# this script pulls the evergreen-ci/lobster repo and copies the files so that
# they can be served by evergreen
# it should only be run from the base evergreen directory
[[ -z "${LOBSTER_TEMP_DIR}" ]] && echo "missing environment variable LOBSTER_TEMP_DIR" && exit 1
[[ -z "${EVGHOME}" ]] && echo "missing environment variable EVGHOME" && exit 1

set -o errexit
set -o xtrace

LOBSTER_REPO=https://github.com/evergreen-ci/lobster.git
LOBSTER_TEMP_ASSETS_DIR="$LOBSTER_TEMP_DIR/build"
LOBSTER_STATIC_DIR="$LOBSTER_TEMP_ASSETS_DIR/static"
LOBSTER_HTML="$LOBSTER_TEMP_ASSETS_DIR/index.html"
EVG_LOBSTER_MANIFEST_DIR="$EVGHOME/public/static/lobster"
EVG_LOBSTER_STATIC_DIR="$EVG_LOBSTER_MANIFEST_DIR/static"
EVG_TEMPLATES_DIR="$EVGHOME/service/templates"

# clone lobster repo
mkdir -p $LOBSTER_TEMP_DIR
git clone $LOBSTER_REPO $LOBSTER_TEMP_DIR

# build lobster
cd $LOBSTER_TEMP_DIR
npm install
# env variables used during lobster build step
export PUBLIC_URL=/static/lobster
export REACT_APP_LOGKEEPER_BASE=https://logkeeper.mongodb.org
export REACT_APP_EVERGREEN_BASE=https://evergreen.mongodb.com
#
npm run build 

# delete lobster static files in evergreen repo
rm -rf $EVG_LOBSTER_STATIC_DIR
mkdir -p $EVG_LOBSTER_STATIC_DIR

# copy files from lobster to evergreen
cp -R $LOBSTER_STATIC_DIR/ $EVG_LOBSTER_STATIC_DIR/
cp $LOBSTER_HTML "$LOBSTER_TEMP_DIR/temp.html"
cp "$LOBSTER_TEMP_ASSETS_DIR/manifest.json" $EVG_LOBSTER_MANIFEST_DIR
# surround the html with go template tags
cd $EVG_TEMPLATES_DIR
echo -e "{{define \"base\"}}\n$(cat $LOBSTER_TEMP_DIR/temp.html)" > lobster.html
echo "{{end}}" >> lobster.html
echo "finished updating lobster"
