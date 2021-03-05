#!/bin/bash

# this script pulls the evergreen-ci/lobster repo and copies the files so that
# they can be served by evergreen
# it should only be run from the base evergreen directory
if [ -n "$LOBSTER_TEMP_DIR" ] && [ -n "$LOBSTER_TEMP_DIR" ]; then
  set -o errexit
  set -o xtrace

  echo $LOBSTER_TEMP_DIR
  LOBSTER_REPO=https://github.com/evergreen-ci/lobster.git
  LOBSTER_TEMP_ASSETS_DIR="$LOBSTER_TEMP_DIR/build"
  LOBSTER_STATIC_DIR="$LOBSTER_TEMP_ASSETS_DIR/static"
  LOBSTER_HTML=index.html
  EVG_LOB_MANIFEST_DIR="$EVGHOME/public/static/lobster"
  EVG_LOB_STATIC_DIR="$EVG_LOB_MANIFEST_DIR/static"
  EVERGREEN_TEMPLATES_DIR="$EVGHOME/service/templates"

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
  rm -rf $EVG_LOB_STATIC_DIR
  mkdir -p $EVG_LOB_STATIC_DIR

  # copy files from lobster to evergreen
  cp -R $LOBSTER_STATIC_DIR/ $EVG_LOB_STATIC_DIR/
  cp "$LOBSTER_TEMP_ASSETS_DIR/$LOBSTER_HTML" "$LOBSTER_TEMP_DIR/temp.html"
  cp "$LOBSTER_TEMP_ASSETS_DIR/manifest.json" $EVG_LOB_MANIFEST_DIR
  # surround the html with go template tags
  cd $EVERGREEN_TEMPLATES_DIR
  echo -e "{{define \"base\"}}\n$(cat $LOBSTER_TEMP_DIR/temp.html)" > lobster.html
  echo "{{end}}" >> lobster.html
  echo "finished updating lobster"
else
  echo "FAILURE: EVGHOME or LOBSTER_TEMP_DIR not set"
fi
