# This script builds the agent using gccgo. 
# Requires go version 1.4 and gccgo 5.

# Currently only supports building the agent for the local platform.
# This script does not copy the resulting binary to an agent platform folder.

. ./set_gopath.sh
go build -compiler gccgo -o main -ldflags "-X github.com/evergreen-ci/evergreen.BuildRevision=`git rev-parse HEAD`" agent/main/agent.go;

