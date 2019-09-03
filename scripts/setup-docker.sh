#!/usr/bin/env bash

set -xeuo pipefail

echo "Installing docker repository and client...."

export PATH="$PATH:/usr/sbin"

# avoid problems where docker is already installed
if ! type docker &> /dev/null ; then
  if [ -f /var/lib/pacman/db.lck ] ; then
    echo "sleeping for 10s to avoid pacman lockfile"
    sleep 10
  fi
  sudo pacman --refresh --noconfirm -S community/docker
fi

type docker

systemctl start docker.service

docker version --format '{{.Server.Version}}'