name := jasper
buildDir := build
srcFiles := $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -name "*_test.go" -not -path "*\#*" -not -path "./vendor/*")
testFiles := $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -path "*\#*")
testPackages := $(name) cli remote options mock
allPackages := $(testPackages) internal-executor remote-internal testutil benchmarks util
lintPackages := $(allPackages)
projectPath := github.com/mongodb/jasper

# start environment setup
gobin := $(GO_BIN_PATH)
ifeq ($(gobin),)
gobin := go
endif
gopath := $(GOPATH)
gocache := $(abspath $(buildDir)/.cache)
goroot := $(GOROOT)
ifeq ($(OS),Windows_NT)
gocache := $(shell cygpath -m $(gocache))
gopath := $(shell cygpath -m $(gopath))
goroot := $(shell cygpath -m $(goroot))
endif

export GOPATH := $(gopath)
export GOCACHE := $(gocache)
export GOROOT := $(goroot)
# end environment setup


# Ensure the build directory exists, since most targets require it.
$(shell mkdir -p $(buildDir))


_compilePackages := $(subst $(name),,$(subst -,/,$(foreach target,$(allPackages),./$(target))))
testOutput := $(foreach target,$(testPackages),$(buildDir)/output.$(target).test)
lintOutput := $(foreach target,$(lintPackages),$(buildDir)/output.$(target).lint)
coverageOutput := $(foreach target,$(testPackages),$(buildDir)/output.$(target).coverage)
coverageHtmlOutput := $(foreach target,$(testPackages),$(buildDir)/output.$(target).coverage.html)


compile $(buildDir): $(srcFiles)
	$(gobin) build $(_compilePackages)
compile-base:
	$(gobin) build ./

# convenience targets for runing tests and coverage tasks on a
# specific package.
test-%: $(buildDir)/output.%.test
	
coverage-%: $(buildDir)/output.%.coverage
	
html-coverage-%: $(buildDir)/output.%.coverage.html
	
lint-%: $(buildDir)/output.%.lint
	
# end convienence targets

# start lint setup targets
lintDeps := $(buildDir)/run-linter $(buildDir)/golangci-lint
$(buildDir)/golangci-lint:
	@curl --retry 10 --retry-max-time 60 -sSfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b $(buildDir) v1.30.0 >/dev/null 2>&1
$(buildDir)/run-linter: cmd/run-linter/run-linter.go $(buildDir)/golangci-lint
	@$(gobin) build -o $@ $<
# end lint setup targets

# benchmark setup targets
$(buildDir)/run-benchmarks: cmd/run-benchmarks/run_benchmarks.go
	$(gobin) build -o $@ $<
# end benchmark setup targets

# start cli targets
$(name) cli: $(buildDir)/$(name)
$(buildDir)/$(name): cmd/$(name)/$(name).go $(srcFiles)
	$(gobin) build -o $@ $<
# end cli targets

# start test and coverage artifacts
#    This varable includes everything that the tests actually need to
#    run. (The "build" target is intentional and makes these targetsb
#    rerun as expected.)
testArgs := -v
ifneq (,$(RUN_TEST))
testArgs += -run='$(RUN_TEST)'
endif
ifneq (,$(RUN_COUNT))
testArgs += -count=$(RUN_COUNT)
endif
ifeq (,$(DISABLE_COVERAGE))
testArgs += -cover
endif
ifneq (,$(RACE_DETECTOR))
testArgs += -race
endif
ifneq (,$(SKIP_LONG))
testArgs += -short
endif
# test execution and output handlers
$(buildDir)/output.%.test: .FORCE
	$(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) | tee $@
	@!( grep -s -q "^FAIL" $@ && grep -s -q "^WARNING: DATA RACE" $@)
	@(grep -s -q "^PASS" $@ || grep -s -q "no test files" $@)
$(buildDir)/output.%.coverage: .FORCE
	$(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) -covermode=count -coverprofile $@ | tee $(buildDir)/output.$*.test
	@-[ -f $@ ] && $(gobin) tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/output.%.coverage.html: $(buildDir)/output.%.coverage .FORCE
	$(gobin) tool cover -html=$< -o $@
#  targets to generate gotest output from the linter.
# We have to handle the PATH specially for CI, because if the PATH has a different version of Go in it, it'll break.
$(buildDir)/output.%.lint: $(buildDir)/run-linter .FORCE
	@$(if $(GO_BIN_PATH), PATH="$(shell dirname $(GO_BIN_PATH)):$(PATH)") ./$< --output=$@ --lintBin=$(buildDir)/golangci-lint --packages='$*'
#  targets to process and generate coverage reports
# end test and coverage artifacts

# user-facing targets for basic build and development operations
proto:
	@mkdir -p remote/internal
	protoc --go_out=plugins=grpc:remote/internal *.proto
lint:$(lintOutput)
test:$(testOutput)
benchmarks: $(buildDir)/run-benchmarks .FORCE
	./$(buildDir)/run-benchmarks $(run-benchmark)
coverage: $(coverageOutput)
coverage-html: $(coverageHtmlOutput)
phony += compile lint test coverage coverage-html docker-setup docker-cleanup
.PHONY: $(phony) .FORCE
.PRECIOUS: $(coverageOutput) $(coverageHtmlOutput) $(lintOutput) $(testOutput)
# end front-ends
#
# Docker-related
docker_image := $(DOCKER_IMAGE)
ifeq ($(docker_image),)
	docker_image := "ubuntu"
endif

docker-setup:
ifeq (,$(SKIP_DOCKER_TESTS))
	docker pull $(docker_image)
endif

docker-cleanup:
ifeq (,$(SKIP_DOCKER_TESTS))
	docker rm -f $(docker ps -a -q)
	docker rmi -f $(docker_image)
endif
# end Docker

.FORCE:

clean:
	rm -rf $(buildDir)/$(name) $(lintDeps) $(buildDir)/run-benchmarks $(buildDir)/run-linter *.pb.go

clean-results:
	rm -rf $(buildDir)/output.*

vendor-clean:
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/google/uuid/
	rm -rf vendor/github.com/mongodb/amboy/vendor/gopkg.in/yaml.v2/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/google/uuid/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/urfave/cli/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/gopkg.in/yaml.v2/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/github.com/evergreen-ci/gimlet/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/google.golang.org/grpc/
	rm -rf vendor/github.com/evergreen-ci/certdepot/mgo_depot.go
	rm -rf vendor/github.com/evergreen-ci/certdepot/vendor/gopkg.in/mgo.v2/
	rm -rf vendor/github.com/evergreen-ci/certdepot/vendor/go.mongodb.org/mongo-driver/
	rm -rf vendor/github.com/evergreen-ci/certdepot/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/evergreen-ci/certdepot/vendor/github.com/square/certstrap/vendor/golang.org/x/crypto
	rm -rf vendor/github.com/evergreen-ci/certdepot/vendor/github.com/square/certstrap/vendor/golang.org/x/sys/
	rm -rf vendor/github.com/evergreen-ci/certdepot/vendor/github.com/square/certstrap/vendor/github.com/urfave/cli/
	rm -rf vendor/github.com/evergreen-ci/certdepot/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/certdepot/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/evergreen-ci/gimlet/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/mongodb/amboy/vendor/golang.org/x/tools/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/urfave/cli/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/mongodb/amboy/vendor/gopkg.in/mgo.v2/
	rm -rf vendor/github.com/mongodb/amboy/vendor/go.mongodb.org/mongo-driver/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/montanaflynn/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/mongodb/grip/vendor/golang.org/x/sys/
	rm -rf vendor/github.com/mongodb/grip/vendor/golang.org/x/oauth2/
	rm -rf vendor/github.com/mongodb/grip/buildscripts/
	rm -rf vendor/github.com/evergreen-ci/bond/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/bond/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/evergreen-ci/bond/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/bond/vendor/github.com/mholt/archiver/
	rm -rf vendor/github.com/evergreen-ci/bond/vendor/github.com/mongodb/amboy/
	rm -rf vendor/github.com/evergreen-ci/bond/vendor/github.com/satori/go.uuid/
	rm -rf vendor/github.com/evergreen-ci/lru/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/lru/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/mholt/archiver/rar.go
	rm -rf vendor/github.com/mholt/archiver/tarbz2.go
	rm -rf vendor/github.com/mholt/archiver/tarlz4.go
	rm -rf vendor/github.com/mholt/archiver/tarsz.go
	rm -rf vendor/github.com/mholt/archiver/tarxz.go
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/montanaflynn/stats/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/pkg/errors/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/stretchr/testify/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/golang.org/x/crypto/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/golang.org/x/net/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/golang.org/x/sys/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/golang.org/x/text/
	rm -rf vendor/go.mongodb.org/mongo-driver/data/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/evergreen-ci/aviation/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/golang/protobuf/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/mongodb/amboy/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/evergreen-ci/pail/vendor/gopkg.in/mgo.v2/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/mongodb/mongo-go-driver/mongo/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/evergreen-ci/pail/vendor/go.mongodb.org/mongo-driver/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/mongodb/ftdc/vendor/go.mongodb.org/mongo-driver/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/golang.org/x/net/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/golang.org/x/sys/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/golang.org/x/text/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/google.golang.org/genproto/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/google.golang.org/grpc/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/gopkg.in/yaml.v2/
	rm -rf vendor/github.com/docker/docker/vendor/github.com/containerd/cgroups/
	rm -rf vendor/github.com/docker/docker/vendor/github.com/coreos/go-systemd/
	rm -rf vendor/github.com/docker/docker/vendor/github.com/godbus/dbus/
	rm -rf vendor/github.com/docker/docker/vendor/github.com/gogo/protobuf/
	rm -rf vendor/github.com/docker/docker/vendor/github.com/google/shlex/
	rm -rf vendor/github.com/docker/docker/vendor/github.com/google/uuid/
	rm -rf vendor/github.com/docker/docker/vendor/golang.org/x/crypto/
	rm -rf vendor/github.com/docker/docker/vendor/golang.org/x/net/
	rm -rf vendor/github.com/docker/docker/vendor/golang.org/x/sys/
	rm -rf vendor/github.com/docker/docker/vendor/golang.org/x/text/
	rm -rf vendor/github.com/docker/docker/vendor/google.golang.org/genproto/
	rm -rf vendor/github.com/docker/docker/vendor/google.golang.org/grpc/
	rm -rf vendor/google.golang.org/genproto/googleapis/ads/
	rm -rf vendor/google.golang.org/genproto/googleapis/analytics/
	rm -rf vendor/google.golang.org/genproto/googleapis/api/
	rm -rf vendor/google.golang.org/genproto/googleapis/appengine/
	rm -rf vendor/google.golang.org/genproto/googleapis/area120/
	rm -rf vendor/google.golang.org/genproto/googleapis/assistant/
	rm -rf vendor/google.golang.org/genproto/googleapis/bigtable/
	rm -rf vendor/google.golang.org/genproto/googleapis/bytestream/
	rm -rf vendor/google.golang.org/genproto/googleapis/chromeos/
	rm -rf vendor/google.golang.org/genproto/googleapis/cloud/
	rm -rf vendor/google.golang.org/genproto/googleapis/container/
	rm -rf vendor/google.golang.org/genproto/googleapis/datastore/
	rm -rf vendor/google.golang.org/genproto/googleapis/devtools/
	rm -rf vendor/google.golang.org/genproto/googleapis/example/
	rm -rf vendor/google.golang.org/genproto/googleapis/firebase/
	rm -rf vendor/google.golang.org/genproto/googleapis/firestore/
	rm -rf vendor/google.golang.org/genproto/googleapis/genomics/
	rm -rf vendor/google.golang.org/genproto/googleapis/geo/
	rm -rf vendor/google.golang.org/genproto/googleapis/grafeas/
	rm -rf vendor/google.golang.org/genproto/googleapis/home/
	rm -rf vendor/google.golang.org/genproto/googleapis/iam/
	rm -rf vendor/google.golang.org/genproto/googleapis/identity/
	rm -rf vendor/google.golang.org/genproto/googleapis/logging/
	rm -rf vendor/google.golang.org/genproto/googleapis/longrunning/
	rm -rf vendor/google.golang.org/genproto/googleapis/maps/
	rm -rf vendor/google.golang.org/genproto/googleapis/monitoring/
	rm -rf vendor/google.golang.org/genproto/googleapis/privacy/
	rm -rf vendor/google.golang.org/genproto/googleapis/pubsub/
	rm -rf vendor/google.golang.org/genproto/googleapis/search
	rm -rf vendor/google.golang.org/genproto/googleapis/spanner/
	rm -rf vendor/google.golang.org/genproto/googleapis/storage/
	rm -rf vendor/google.golang.org/genproto/googleapis/storagetransfer/
	rm -rf vendor/google.golang.org/genproto/googleapis/streetview/
	rm -rf vendor/google.golang.org/genproto/googleapis/type/
	rm -rf vendor/google.golang.org/genproto/googleapis/watcher/
	find vendor/ -name "*.gif" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" -o -name "*testdata*" | xargs rm -rf
	find vendor/ -type d -empty | xargs rm -rf
	find vendor/ -type d -name '.git' | xargs rm -rf
