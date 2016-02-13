#!/bin/sh
set -e
echo "installing plugins."
./install_plugins.sh
./build_agent.sh "${1}"
./build_clients.sh "${1}"
./build.sh
mkdir -p build/bin
mkdir -p build/executables
mkdir -p build/clients
mkdir -p build/public
mkdir -p build/ui/templates
mkdir -p build/ui/plugins
cp -R executables/* build/executables/
cp -R clients/* build/clients/
cp -R public/* build/public/
cp -R ui/templates/* build/ui/templates
cp -R ui/plugins/* build/ui/plugins
cp bin/evergreen_api_server build/bin
cp bin/evergreen_ui_server build/bin
cp bin/evergreen_runner build/bin
tar zcvhf ./deploy.tgz build
