# start project configuration
name := evergreen
buildDir := bin
tmpDir := $(abspath $(buildDir)/tmp)
nodeDir := public
packages := $(name) agent operations cloud cloud-userdata command db util plugin units graphql
packages += thirdparty auth scheduler model validator service repotracker
packages += model-patch model-artifact model-host model-build model-event model-task model-user model-distro model-manifest model-testresult
packages += rest-client rest-data rest-route rest-model migrations trigger model-alertrecord model-notification model-stats model-reliability
lintOnlyPackages := testutil model-manifest
orgPath := github.com/evergreen-ci
projectPath := $(orgPath)/$(name)
# end project configuration


# override the go binary path if set
ifneq (,$(GO_BIN_PATH))
  gobin := $(GO_BIN_PATH)
 else
  gobin := $(shell if [ -x /opt/golang/go1.13/bin/go ]; then echo /opt/golang/go1.13/bin/go; fi)
  ifeq (,$(gobin))
    gobin := go
  endif
endif

ifneq (,$(LEGACY_GO_BIN_PATH))
  legacyGobin := $(LEGACY_GO_BIN_PATH)
 else
  legacyGobin := $(shell if [ -x /opt/golang/go1.9/bin/go ]; then echo /opt/golang/go1.9/bin/go; fi)
  ifeq (,$(legacyGobin))
    legacyGobin := go
  endif
endif
# end gibinary settings


# start evergreen specific configuration

unixPlatforms := linux_amd64 darwin_amd64 $(if $(STAGING_ONLY),,linux_386 linux_s390x linux_arm64 linux_ppc64le linux_amd64_legacy)
windowsPlatforms := windows_amd64 $(if $(STAGING_ONLY),,windows_386)


goos := $(shell $(gobin) env GOOS)
goarch := $(shell $(gobin) env GOARCH)

clientBuildDir := clients


clientBinaries := $(foreach platform,$(unixPlatforms),$(clientBuildDir)/$(platform)/evergreen)
clientBinaries += $(foreach platform,$(windowsPlatforms),$(clientBuildDir)/$(platform)/evergreen.exe)

clientSource := cmd/evergreen/evergreen.go
uiFiles := $(shell find public/static -not -path "./public/static/app" -name "*.js" -o -name "*.css" -o -name "*.html")

distArtifacts :=  ./public ./service/templates
distContents := $(clientBinaries) $(distArtifacts)
srcFiles := makefile $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -name "*_test.go" -not -path "./scripts/*" -not -path "*\#*")
testSrcFiles := makefile $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -path "*\#*")
currentHash := $(shell git rev-parse HEAD)
agentVersion := $(shell grep "AgentVersion" config.go | tr -d '\tAgentVersion = ' | tr -d '"')
ldFlags := $(if $(DEBUG_ENABLED),,-w -s )-X=github.com/evergreen-ci/evergreen.BuildRevision=$(currentHash)
karmaFlags := $(if $(KARMA_REPORTER),--reporters $(KARMA_REPORTER),)
smokeFile := $(if $(SMOKE_TEST_FILE),--test-file $(SMOKE_TEST_FILE),)
# end evergreen specific configuration

gopath := $(GOPATH)
ifeq ($(OS),Windows_NT)
ifneq (,$(gopath))
gopath := $(shell cygpath -m $(gopath))
endif
endif

######################################################################
##
## Build rules and instructions for building evergreen binaries and targets.
##
######################################################################


# start rules for building services and clients
ifeq ($(OS),Windows_NT)
localClientBinary := $(clientBuildDir)/$(goos)_$(goarch)/evergreen.exe
else
localClientBinary := $(clientBuildDir)/$(goos)_$(goarch)/evergreen
endif
cli:$(localClientBinary)
clis:$(clientBinaries)
$(clientBuildDir)/%/evergreen $(clientBuildDir)/%/evergreen.exe:$(buildDir)/build-cross-compile $(srcFiles)
	@./$(buildDir)/build-cross-compile -buildName=$* -ldflags="$(ldFlags)" -legacyGoBinary="$(legacyGobin)" -goBinary="$(gobin)" $(if $(RACE_DETECTOR),-race ,)-directory=$(clientBuildDir) -source=$(clientSource) -output=$@
phony += cli clis
# end client build directives



# start smoke test specific rules
$(buildDir)/load-smoke-data:cmd/load-smoke-data/load-smoke-data.go
	$(gobin) build -ldflags="-w" -o $@ $<
$(buildDir)/set-var:cmd/set-var/set-var.go
	$(gobin) build -o $@ $<
$(buildDir)/set-project-var:cmd/set-project-var/set-project-var.go
	$(gobin) build -o $@ $<
set-var:$(buildDir)/set-var
set-project-var:$(buildDir)/set-project-var
set-smoke-vars:$(buildDir)/.load-smoke-data
	@./bin/set-project-var -dbName mci_smoke -key aws_key -value $(AWS_KEY)
	@./bin/set-project-var -dbName mci_smoke -key aws_secret -value $(AWS_SECRET)
	@./bin/set-var -dbName=mci_smoke -collection=hosts -id=localhost -key=agent_revision -value=$(agentVersion)
load-smoke-data:$(buildDir)/.load-smoke-data
load-local-data:$(buildDir)/.load-local-data
$(buildDir)/.load-smoke-data:$(buildDir)/load-smoke-data
	./$<
	@touch $@
$(buildDir)/.load-local-data:$(buildDir)/load-smoke-data
	./$< --path testdata/local --dbName evergreen_local
	@touch $@
smoke-test-agent-monitor:$(localClientBinary) load-smoke-data
	./$< service deploy start-evergreen --web --binary ./$< &
	./$< service deploy start-evergreen --monitor --binary ./$< --client_url ${CLIENT_URL} &
	./$< service deploy test-endpoints --check-build --username admin --key abb623665fdbf368a1db980dde6ee0f0 $(smokeFile) || (pkill -f $<; exit 1)
	pkill -f $<
smoke-test-task:$(localClientBinary) load-smoke-data
	./$< service deploy start-evergreen --web --binary ./$< &
	./$< service deploy start-evergreen --agent --binary ./$< &
	./$< service deploy test-endpoints --check-build --username admin --key abb623665fdbf368a1db980dde6ee0f0 $(smokeFile) || (pkill -f $<; exit 1)
	pkill -f $<
smoke-test-endpoints:$(localClientBinary) load-smoke-data
	./$< service deploy start-evergreen --web --binary ./$< &
	./$< service deploy test-endpoints --username admin --key abb623665fdbf368a1db980dde6ee0f0 $(smokeFile) || (pkill -f $<; exit 1)
	pkill -f $<
local-evergreen:$(localClientBinary) load-local-data
	./$< service deploy start-local-evergreen
# end smoke test rules

######################################################################
##
## Build, Test, and Dist targets and mechisms.
##
######################################################################

# most of the targets and variables in this section are generic
# instructions for go programs of all kinds, and are not particularly
# specific to evergreen; though the dist targets are more specific than the rest.

# start dependency installation tools
#   implementation details for being able to lazily install dependencies.
#   this block has no project specific configuration but defines
#   variables that project specific information depends on
testOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).test)
coverageOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage)
coverageHtmlOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage.html)
$(gopath)/src/%:
	@-[ ! -d $(gopath) ] && mkdir -p $(gopath) || true
	$(gobin) get $(subst $(gopath)/src/,,$@)
$(gopath)/bin:
	mkdir -p $@
# end dependency installation tools


# lint setup targets
$(buildDir)/.lintSetup:$(buildDir)/golangci-lint
	$(gobin) get github.com/evergreen-ci/evg-lint/...
	@mkdir -p $(buildDir)
	@touch $@
$(buildDir)/golangci-lint:
	@curl --retry 10 --retry-max-time 60 -sSfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b $(buildDir) v1.10.2 >/dev/null 2>&1 && touch $@
$(buildDir)/run-linter:cmd/run-linter/run-linter.go $(buildDir)/.lintSetup
	@mkdir -p $(buildDir)
	$(gobin) build -ldflags "-w" -o $@ $<

# end lint setup targets

# generate lint JSON document for evergreen
generate-lint:$(buildDir)/generate-lint.json
$(buildDir)/generate-lint.json:$(buildDir)/generate-lint $(srcFiles)
	./$(buildDir)/generate-lint
$(buildDir)/generate-lint:cmd/generate-lint/generate-lint.go
	$(gobin) build -ldflags "-w" -o  $@ $<
# end generate lint

# generated config for go tests
go-test-config:$(buildDir)/go-test-config.json
$(buildDir)/go-test-config.json:$(buildDir)/go-test-config
	./$(buildDir)/go-test-config
$(buildDir)/go-test-config:cmd/go-test-config/make-config.go
	GOPATH=$(shell dirname $(shell pwd)) $(gobin) build -o $@ $<
#end generated config

# parse a host.create file and set expansions
parse-host-file:$(buildDir)/parse-host-file
	./$(buildDir)/parse-host-file --file $(HOST_FILE)
$(buildDir)/parse-host-file:cmd/parse-host-file/parse-host-file.go
	$(gobin) build -o $@ $<
$(buildDir)/expansions.yml:$(buildDir)/parse-host-file
# end host.create file parsing

# npm setup
$(buildDir)/.npmSetup:
	@mkdir -p $(buildDir)
	cd $(nodeDir) && $(if $(NODE_BIN_PATH),export PATH=${PATH}:$(NODE_BIN_PATH) && ,)npm install
	touch $@
# end npm setup


# distribution targets and implementation
$(buildDir)/build-cross-compile:cmd/build-cross-compile/build-cross-compile.go makefile
	@mkdir -p $(buildDir)
	@GOOS="" GOARCH="" $(gobin) build -o $@ $<
	@echo $(gobin) build -o $@ $<
$(buildDir)/make-tarball:cmd/make-tarball/make-tarball.go
	@mkdir -p $(buildDir)
	@GOOS="" GOARCH="" $(gobin) build -o $@ $<
	@echo $(gobin) build -o $@ $<

dist-staging: export STAGING_ONLY := 1
dist-staging:
	make dist
dist:$(buildDir)/dist.tar.gz
$(buildDir)/dist.tar.gz:$(buildDir)/make-tarball $(clientBinaries) $(uiFiles)
	./$< --name $@ --prefix $(name) $(foreach item,$(distContents),--item $(item)) --exclude "public/node_modules" --exclude "clients/.cache"
# end main build


# userfacing targets for basic build and development operations
build:cli
build-alltests:$(testBin)
build-all:build-alltests build
lint:$(foreach target,$(packages) $(lintOnlyPackages),$(buildDir)/output.$(target).lint)
test:$(foreach target,$(packages),test-$(target))
js-test:$(buildDir)/.npmSetup
	cd $(nodeDir) && $(if $(NODE_BIN_PATH),export PATH=${PATH}:$(NODE_BIN_PATH) && ,)./node_modules/.bin/karma start static/js/tests/conf/karma.conf.js $(karmaFlags)
coverage:$(coverageOutput)
coverage-html:$(coverageHtmlOutput)
list-tests:
	@echo -e "test targets:" $(foreach target,$(packages),\\n\\ttest-$(target))
phony += lint lint-deps build test coverage coverage-html list-tests
.PRECIOUS:$(testOutput) $(coverageOutput) $(coverageHtmlOutput)
.PRECIOUS:$(foreach target,$(packages),$(buildDir)/output.$(target).lint)
.PRECIOUS:$(buildDir)/output.lint
# end front-ends


# start vendoring configuration
vendor-clean:
	rm -rf vendor/github.com/docker/docker/vendor/github.com/Microsoft/go-winio/
	rm -rf vendor/github.com/docker/docker/vendor/github.com/docker/go-connections/
	rm -rf vendor/github.com/docker/docker/vendor/golang.org/x/net/
	rm -rf vendor/github.com/docker/docker/vendor/golang.org/x/crypto/
	rm -rf vendor/github.com/docker/docker/vendor/golang.org/x/sys/
	rm -rf vendor/github.com/docker/docker/vendor/golang.org/x/text/
	rm -rf vendor/github.com/docker/docker/vendor/golang.org/x/oauth2/
	rm -rf vendor/github.com/docker/docker/vendor/google.golang.org/genproto/
	rm -rf vendor/github.com/docker/docker/vendor/google.golang.org/grpc/
	rm -rf vendor/github.com/evergreen-ci/certdepot/vendor/github.com/mongodb/anser/
	rm -rf vendor/github.com/evergreen-ci/certdepot/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/certdepot/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/certdepot/vendor/github.com/square/certstrap/
	rm -rf vendor/github.com/evergreen-ci/certdepot/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/evergreen-ci/certdepot/vendor/go.mongodb.org/mongo-driver/
	rm -rf vendor/github.com/evergreen-ci/certdepot/vendor/gopkg.in/mgo.v2/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/go.mongodb.org/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/gopkg.in/yaml.v2/
	rm -rf vendor/github.com/evergreen-ci/go-test2json/vendor
	rm -rf vendor/github.com/evergreen-ci/pail/testdata/
	rm -rf vendor/github.com/evergreen-ci/pail/vendor/github.com/aws/aws-sdk-go/
	rm -rf vendor/github.com/evergreen-ci/pail/vendor/github.com/mitchellh/go-homedir/
	rm -rf vendor/github.com/evergreen-ci/pail/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/pail/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/pail/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/evergreen-ci/pail/vendor/go.mongodb.org/mongo-driver/
	rm -rf vendor/github.com/evergreen-ci/pail/vendor/gopkg.in/mgo.v2/
	rm -rf vendor/github.com/evergreen-ci/timber/vendor/github.com/evergreen-ci/aviation/vendor/github.com/evergreen-ci/gimlet/
	rm -rf vendor/github.com/evergreen-ci/timber/vendor/github.com/golang/protobuf/
	rm -rf vendor/github.com/evergreen-ci/timber/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/timber/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/timber/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/evergreen-ci/timber/vendor/github.com/PuerkitoBio/rehttp/
	rm -rf vendor/github.com/evergreen-ci/timber/vendor/go.mongodb.org/mongo-driver/
	rm -rf vendor/github.com/evergreen-ci/timber/vendor/golang.org/x/net/
	rm -rf vendor/github.com/evergreen-ci/timber/vendor/golang.org/x/sys/
	rm -rf vendor/github.com/evergreen-ci/timber/vendor/golang.org/x/text/
	rm -rf vendor/github.com/evergreen-ci/timber/vendor/google.golang.org/genproto/
	rm -rf vendor/github.com/evergreen-ci/timber/vendor/google.golang.org/grpc/
	rm -rf vendor/github.com/gorilla/csrf/vendor/github.com/gorilla/context/
	rm -rf vendor/github.com/gorilla/csrf/vendor/github.com/pkg/
	rm -rf vendor/github.com/mholt/archiver/rar.go
	rm -rf vendor/github.com/mholt/archiver/tarbz2.go
	rm -rf vendor/github.com/mholt/archiver/tarlz4.go
	rm -rf vendor/github.com/mholt/archiver/tarsz.go
	rm -rf vendor/github.com/mholt/archiver/tarxz.go
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/aws/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/evergreen-ci/gimlet/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/urfave/cli/
	rm -rf vendor/github.com/mongodb/amboy/vendor/go.mongodb.org/mongo-driver/
	rm -rf vendor/github.com/mongodb/amboy/vendor/gonum.org/v1/gonum
	rm -rf vendor/github.com/mongodb/amboy/vendor/gopkg.in/mgo.v2/
	rm -rf vendor/github.com/mongodb/amboy/vendor/gopkg.in/yaml.v2/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/gopkg.in/mgo.v2
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/pkg/errors
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/go.mongodb.org/mongo-driver
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/mongodb/grip
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/evergreen-ci/birch
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/papertrail/go-tail/vendor/golang.org/x/sys/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/google/go-github/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/shirou/gopsutil/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/mongodb/grip/vendor/golang.org/x/oauth2/
	rm -rf vendor/github.com/mongodb/grip/vendor/golang.org/x/sys/
	rm -rf vendor/github.com/mongodb/jasper/harness.go
	rm -rf vendor/github.com/mongodb/jasper/cmd/run-benchmarks/
	rm -rf vendor/github.com/mongodb/jasper/vendor/github.com/evergreen-ci/poplar/
	rm -rf vendor/github.com/mongodb/jasper/vendor/gopkg.in/mgo.v2/
	rm -rf vendor/github.com/mongodb/jasper/vendor/gopkg.in/yaml.v2/
	rm -rf vendor/github.com/mongodb/jasper/vendor/github.com/evergreen-ci/certdepot
	rm -rf vendor/github.com/mongodb/jasper/vendor/github.com/evergreen-ci/gimlet/
	rm -rf vendor/github.com/mongodb/jasper/vendor/github.com/evergreen-ci/timber/
	rm -rf vendor/github.com/mongodb/jasper/vendor/github.com/evergreen-ci/birch/
	rm -rf vendor/github.com/mongodb/jasper/vendor/github.com/evergreen-ci/shrub/
	rm -rf vendor/github.com/mongodb/jasper/vendor/github.com/evergreen-ci/utility/
	rm -rf vendor/github.com/mongodb/jasper/vendor/github.com/golang/protobuf/
	rm -rf vendor/github.com/mongodb/jasper/vendor/github.com/mholt/archiver/
	rm -rf vendor/github.com/mongodb/jasper/vendor/github.com/mongodb/amboy/
	rm -rf vendor/github.com/mongodb/jasper/vendor/github.com/mongodb/ftdc
	rm -rf vendor/github.com/mongodb/jasper/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/mongodb/jasper/vendor/github.com/pkg/
	rm -rf vendor/github.com/mongodb/jasper/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/mongodb/jasper/vendor/github.com/PuerkitoBio/rehttp/
	rm -rf vendor/github.com/mongodb/jasper/vendor/github.com/docker/docker/
	rm -rf vendor/github.com/mongodb/jasper/vendor/go.mongodb.org/mongo-driver/
	rm -rf vendor/github.com/mongodb/jasper/vendor/golang.org/x/net/
	rm -rf vendor/github.com/mongodb/jasper/vendor/golang.org/x/sys/
	rm -rf vendor/github.com/mongodb/jasper/vendor/golang.org/x/text/
	rm -rf vendor/github.com/mongodb/jasper/vendor/golang.org/x/crypto/
	rm -rf vendor/github.com/mongodb/jasper/vendor/golang.org/x/oauth2/
	rm -rf vendor/github.com/mongodb/jasper/vendor/google.golang.org/genproto/
	rm -rf vendor/github.com/mongodb/jasper/vendor/google.golang.org/grpc/
	rm -rf vendor/github.com/smartystreets/goconvey/web/
	rm -rf vendor/github.com/square/certstrap/vendor/github.com/urfave/cli/
	rm -rf vendor/github.com/square/certstrap/vendor/golang.org/x/sys/unix/
	rm -rf vendor/github.com/square/certstrap/vendor/golang.org/x/sys/windows/
	rm -rf vendor/go.mongodb.org/mongo-driver/data/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/davecgh
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/montanaflynn
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/pmezard
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/stretchr
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/golang.org/x/crypto
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/golang.org/x/net
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/golang.org/x/text
	rm -rf vendor/gopkg.in/mgo.v2/.git/
	rm -rf vendor/gopkg.in/mgo.v2/harness/
	rm -rf vendor/gopkg.in/mgo.v2/internal/json/testdata
	rm -rf vendor/gopkg.in/mgo.v2/testdb/
	rm -rf vendor/gopkg.in/mgo.v2/testserver/
	rm -rf vendor/gopkg.in/mgo.v2/txn/
	rm -rf vendor/github.com/mongodb/anser/vendor/gopkg.in/mgo.v2
	rm -rf vendor/github.com/mongodb/anser/vendor/github.com/stretchr/testify
	rm -rf vendor/github.com/mongodb/anser/vendor/go.mongodb.org/mongo-driver
	rm -rf vendor/github.com/mongodb/anser/vendor/github.com/mongodb/grip
	rm -rf vendor/github.com/mongodb/anser/vendor/github.com/pkg/errors
	rm -rf vendor/github.com/mongodb/anser/vendor/github.com/tychoish/tarjan
	rm -rf vendor/github.com/mongodb/anser/vendor/github.com/mongodb/ftdc
	rm -rf vendor/github.com/mongodb/anser/vendor/github.com/mongodb/amboy
	rm -rf vendor/github.com/mongodb/anser/vendor/github.com/evergreen-ci/birch
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/google/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/google/uuid/
	rm -rf vendor/github.com/mongodb/jasper/vendor/github.com/google/uuid/
	rm -rf vendor/github.com/vmware/govmomi/vendor/github.com/google/uuid
	rm -rf vendor/github.com/shirou/gopsutil/host/host_linux_riscv64.go
	mkdir -p vendor/github.com/vektah/gqlparser/v2 && find vendor/github.com/vektah/gqlparser -maxdepth 1 -mindepth 1 -not -name "v2" -exec mv {} vendor/github.com/vektah/gqlparser/v2 \;
	find vendor/ -name "*.gif" -o -name "*.jpg" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" | xargs rm -rf
phony += vendor-clean
$(buildDir)/run-glide:cmd/revendor/run-glide.go
	$(gobin) build -o $@ $<
run-glide:$(buildDir)/run-glide
	$(buildDir)/run-glide $(if $(VENDOR_REVISION),--revision $(VENDOR_REVISION),) $(if $(VENDOR_PKG),--package $(VENDOR_PKG) ,)
revendor:
ifneq ($(VENDOR_REVISION),)
revendor:run-glide vendor-clean
else
revendor:
endif
# do not use this target - it's a workaround to be able to vendor+pin a specific version
# of goimports when running test-rest-model in evergreen. As long as you have GOROOT
# set to the directory containing your same version (1.9+) of the go binary, goimports
# will work without this workaround
get-go-imports:
	GOPATH=$(gopath) $(gobin) get -u golang.org/x/tools/imports
	cd $(gopath)/src/golang.org/x/tools && git reset 727c06e3f111405bd52063f6120c7d72c3ba896e --hard
# end vendoring tooling configuration


# convenience targets for runing tests and coverage tasks on a
# specific package.
test-%:$(buildDir)/output.%.test
	@grep -s -q -e "^PASS" $< && ! grep -s -q "^WARNING: DATA RACE" $<
dlv-%:$(buildDir)/output-dlv.%.test
	@grep -s -q -e "^PASS" $< && ! grep -s -q "^WARNING: DATA RACE" $<
coverage-%:$(buildDir)/output.%.coverage
	@grep -s -q -e "^PASS" $(subst coverage,test,$<)
html-coverage-%:$(buildDir)/output.%.coverage $(buildDir)/output.%.coverage.html
	@grep -s -q -e "^PASS" $(subst coverage,test,$<)
lint-%:$(buildDir)/output.%.lint
	@grep -v -s -q "^--- FAIL" $<
# end convienence targets


# start test and coverage artifacts
#    This varable includes everything that the tests actually need to
#    run. (The "build" target is intentional and makes these targetsb
#    rerun as expected.)
testRunDeps := $(name)
testArgs := -v
dlvArgs := -test.v
testRunEnv := EVGHOME=$(shell pwd) GOCONVEY_REPORTER=silent GOPATH=$(gopath)
ifeq ($(OS),Windows_NT)
testRunEnv := EVGHOME=$(shell cygpath -m `pwd`) GOPATH=$(gopath)
endif
ifneq (,$(SETTINGS_OVERRIDE))
testRunEnv += SETTINGS_OVERRIDE=$(SETTINGS_OVERRIDE)
endif
ifneq (,$(TMPDIR))
testRunEnv += TMPDIR=$(TMPDIR)
else
testRunEnv += TMPDIR=$(tmpDir)
endif
ifneq (,$(RUN_TEST))
testArgs += -run='$(RUN_TEST)'
dlvArgs += -test.run='$(RUN_TEST)'
endif
ifneq (,$(SKIP_LONG))
testArgs += -short
dlvArgs += -test.short
endif
ifneq (,$(RUN_COUNT))
testArgs += -count='$(RUN_COUNT)'
dlvArgs += -test.count='$(RUN_COUNT)'
endif
ifneq (,$(RACE_DETECTOR))
testArgs += -race
dlvArgs += -test.race
endif
ifneq (,$(TEST_TIMEOUT))
testArgs += -timeout=$(TEST_TIMEOUT)
else
testArgs += -timeout=10m
endif
testArgs += -ldflags="$(ldFlags) -X=github.com/evergreen-ci/evergreen/testutil.ExecutionEnvironmentType=test"
#  targets to run any tests in the top-level package
$(buildDir):
	mkdir -p $@
$(tmpDir):$(buildDir)
	mkdir -p $@
$(buildDir)/output.%.test:$(tmpDir) .FORCE
	$(testRunEnv) $(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) 2>&1 | tee $@
$(buildDir)/output-dlv.%.test:$(tmpDir) .FORCE
	$(testRunEnv) dlv test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) -- $(dlvArgs) 2>&1 | tee $@
$(buildDir)/output.%.coverage:$(tmpDir) .FORCE
	$(testRunEnv) $(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) -covermode=count -coverprofile $@ | tee $(buildDir)/output.$*.test
	@-[ -f $@ ] && go tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
#  targets to generate gotest output from the linter.
$(buildDir)/output.%.lint:$(buildDir)/run-linter $(testSrcFiles) .FORCE
	@./$< --output=$@ --lintBin="$(buildDir)/golangci-lint" --customLinters="$(gopath)/bin/evg-lint -set_exit_status" --packages='$*'
$(buildDir)/output.lint:$(buildDir)/run-linter .FORCE
	@./$< --output="$@" --lintBin="$(buildDir)/golangci-lint" --customLinters="$(gopath)/bin/evg-lint -set_exit_status" --packages="$(packages)"
#  targets to process and generate coverage reports
$(buildDir)/output.%.coverage.html:$(buildDir)/output.%.coverage
	$(gobin) tool cover -html=$< -o $@
# end test and coverage artifacts


# clean and other utility targets
clean:
	rm -rf $(lintDeps) $(buildDir)/test.* $(buildDir)/output.* $(clientBuildDir) $(tmpDir)
	rm -rf $(gopath)/pkg/
phony += clean

gqlgen:
	go run vendor/github.com/99designs/gqlgen/main.go
# end dependency targets

# sanitizes a json file by hashing string values. Note that this will not work well with
# string data that only has a subset of valid values
ifneq (,$(multi))
multiarg = --multi
endif
scramble:
	python cmd/scrambled-eggs/scramble.py $(file) $(multiarg)

# mongodb utility targets
mongodb/.get-mongodb:
	rm -rf mongodb
	mkdir -p mongodb
	cd mongodb && curl "$(MONGODB_URL)" -o mongodb.tgz && $(DECOMPRESS) mongodb.tgz && chmod +x ./mongodb-*/bin/*
	cd mongodb && mv ./mongodb-*/bin/* . && rm -rf db_files && rm -rf db_logs && mkdir -p db_files && mkdir -p db_logs
get-mongodb:mongodb/.get-mongodb
	@touch $<
start-mongod:mongodb/.get-mongodb
	./mongodb/mongod --dbpath ./mongodb/db_files --port 27017 --replSet evg --smallfiles --oplogSize 10
	@echo "waiting for mongod to start up"
init-rs:mongodb/.get-mongodb
	./mongodb/mongo --eval 'rs.initiate()'
check-mongod:mongodb/.get-mongodb
	./mongodb/mongo --nodb --eval "assert.soon(function(x){try{var d = new Mongo(\"localhost:27017\"); return true}catch(e){return false}}, \"timed out connecting\")"
	@echo "mongod is up"
# end mongodb targets


# configure special (and) phony targets
.FORCE:
.PHONY:$(phony) .FORCE
.DEFAULT_GOAL:build
