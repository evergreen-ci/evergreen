#!/usr/bin/env bash
set -o errexit
set -o pipefail

# make sure we're in the directory where the script lives
SCRIPT_DIR="$(pwd)"
cd $SCRIPT_DIR
echo "Installing plugins..."

# set the $GOPATH appropriately. as the first entry in the $GOPATH,
# the dependencies will be installed in vendor/
# . ./set_gopath.sh

echo `pwd`
# make sure the Plugins file is there
deps_file="Plugins"
[[ -f "$deps_file" ]] || (echo ">> $deps_file file does not exist." && exit 1)

# make sure go is installed
(go version > /dev/null) || (echo ">> Go is currently not installed or in your PATH" && exit 1)

# iterate over Plugins file dependencies and set
# the specified version on each of them.
while read line; do
  linesplit=`echo $line | sed 's/#.*//;/^\s*$/d' || echo ""`
  [ ! "$linesplit" ] && continue
  (
    cd $SCRIPT_DIR

    linearr=($linesplit)
    package=${linearr[0]}
    version=${linearr[1]}
    pluginconf=${linearr[2]}
    pluginname=${linearr[3]}

    if [[ "${pluginconf}" == "" ]]; then
        echo ">> Error: must specify a plugin config file for ${package}"
        exit 1
    fi

    install_path="$SCRIPT_DIR/vendor/${package}"

    # clean out the install path
    rm -rf $install_path

    [[ -e "$install_path/.git/index.lock" ]] && wait

    echo ">> Getting package "$package""
    mkdir -p $(dirname vendor/$package)
    repoName=$(echo $package | sed 's/github.com//')
    git clone git@github.com:$repoName vendor/$package
    cd $install_path
    git checkout "$version" || { echo ">> Failed to set $package to version $version"; exit 1; }
    echo ">> Set $package to version $version"

    echo ">> Linking in plugin config ${pluginconf} for package ${package}"
    mkdir -p $SCRIPT_DIR/plugin/config
    cd $SCRIPT_DIR/plugin/config
    $(rm $pluginconf || true)
    cp $install_path/config/$pluginconf $pluginconf
    mkdir -p $SCRIPT_DIR/service/plugins/$pluginname

    if [ -d "$install_path/templates/" ]; then
            echo "creating template links to service/plugins/$pluginname"
            # remove existing symlink if its already there
            rm -rf $SCRIPT_DIR/service/plugins/$pluginname/templates
            rsync --verbose --delete -recursive $install_path/templates $SCRIPT_DIR/service/plugins/$pluginname/
    fi
    if [ -d "$install_path/static/" ]; then
            echo "creating static links to service/plugins/$pluginname"
            # remove existing symlink if its already there
            rm -rf $SCRIPT_DIR/service/plugins/$pluginname/static
            rsync --verbose --delete -recursive $install_path/static $SCRIPT_DIR/service/plugins/$pluginname/
    fi

    echo ">> Plugin successfully installed"

  )
done < $deps_file

echo ">> All Done"
