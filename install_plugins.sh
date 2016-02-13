#!/usr/bin/env bash

# make sure we're in the directory where the script lives
SCRIPT_DIR="$(cd "$(dirname ${BASH_SOURCE[0]})" && pwd)"
cd $SCRIPT_DIR
echo "Installing plugins..."

# set the $GOPATH appropriately. as the first entry in the $GOPATH,
# the dependencies will be installed in vendor/
. ./set_gopath.sh

# make sure the Plugins file is there
deps_file="Plugins"
[[ -f "$deps_file" ]] || (echo ">> $deps_file file does not exist." && exit 1)

# make sure go is installed
(go version > /dev/null) ||
  ( echo ">> Go is currently not installed or in your PATH" && exit 1)

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

    if [[ "${pluginconf}" == "" ]]
    then 
        echo ">> Error: must specify a plugin config file for ${package}"
        exit 1
    fi

    install_path="vendor/src/${package%%/...}"

    # clean out the install path
    rm -rf $install_path

    [[ -e "$install_path/.git/index.lock" ||
       -e "$install_path/.hg/store/lock"  ||
       -e "$install_path/.bzr/checkout/lock" ]] && wait

    echo ">> Getting package "$package""
    go get -d "$package"

    cd $install_path
    hg update     "$version" > /dev/null 2>&1 || \
    git checkout  "$version" > /dev/null 2>&1 || \
    bzr revert -r "$version" > /dev/null 2>&1 || \
    #svn has exit status of 0 when there is no .svn
    { [ -d .svn ] && svn update -r "$version" > /dev/null 2>&1; } || \
    { echo ">> Failed to set $package to version $version"; exit 1; }

    echo ">> Set $package to version $version"

    echo ">> Linking in plugin config ${pluginconf} for package ${package}"
    cd $SCRIPT_DIR/plugin/config
    $(rm $pluginconf || true)
    ln -s ../../$install_path/config/$pluginconf $pluginconf
	if [ -d "../../$install_path/templates/" ]; then
		echo "creating template links to ui/plugins/$pluginname"
		mkdir -p $SCRIPT_DIR/ui/plugins/$pluginname
		# remove existing symlink if its already there
		rm $SCRIPT_DIR/ui/plugins/$pluginname/templates || true
		ln -s $SCRIPT_DIR/$install_path/templates/ $SCRIPT_DIR/ui/plugins/$pluginname/
	fi
	if [ -d "../../$install_path/static/" ]; then
		echo "creating static links to ui/plugins/$pluginname"
		mkdir -p $SCRIPT_DIR/ui/plugins/$pluginname
		# remove existing symlink if its already there
		rm $SCRIPT_DIR/ui/plugins/$pluginname/static
		ln -s $SCRIPT_DIR/$install_path/static/ $SCRIPT_DIR/ui/plugins/$pluginname/
	fi
    echo ">> Plugin successfully installed"

  ) 
done < $deps_file

echo ">> All Done"
