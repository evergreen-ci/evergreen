# start project configuration
name := evergreen
buildDir := bin
nodeDir := public
packages := $(name) agent agent-command agent-util agent-internal agent-internal-client agent-internal-testutil operations cloud cloud-userdata
packages += db util plugin units graphql thirdparty thirdparty-docker auth scheduler model validator service repotracker mock
packages += model-annotations model-patch model-artifact model-host model-pod model-pod-definition model-pod-dispatcher model-build model-event model-task model-user model-distro model-manifest model-testresult model-log model-testlog
packages += model-commitqueue model-cache
packages += rest-client rest-data rest-route rest-model migrations trigger model-alertrecord model-notification model-taskstats model-reliability
packages += taskoutput
lintOnlyPackages := api apimodels testutil model-manifest model-testutil service-testutil service-graphql db-mgo db-mgo-bson db-mgo-internal-json rest
lintOnlyPackages += smoke-internal smoke-internal-host smoke-internal-container smoke-internal-agentmonitor smoke-internal-endpoint
testOnlyPackages := service-graphql smoke-internal-host smoke-internal-container smoke-internal-agentmonitor smoke-internal-endpoint # has only test files so can't undergo all operations
orgPath := github.com/evergreen-ci
projectPath := $(orgPath)/$(name)
evghome := $(abspath .)
ifeq ($(OS),Windows_NT)
	evghome := $(shell cygpath -m $(evghome))
endif
lobsterTempDir := $(abspath $(buildDir))/lobster-temp
# end project configuration

# start go runtime settings
gobin := go
ifneq (,$(GOROOT))
gobin := $(GOROOT)/bin/go
endif

goCache := $(GOCACHE)
ifeq (,$(goCache))
goCache := $(abspath $(buildDir)/.cache)
endif
goModCache := $(GOMODCACHE)
ifeq (,$(goModCache))
goModCache := $(abspath $(buildDir)/.mod-cache)
endif
lintCache := $(GOLANGCI_LINT_CACHE)
ifeq (,$(lintCache))
lintCache := $(abspath $(buildDir)/.lint-cache)
endif

ifeq ($(OS),Windows_NT)
gobin := $(shell cygpath $(gobin))
nativeGobin := $(shell cygpath -m $(gobin))
goCache := $(shell cygpath -m $(goCache))
goModCache := $(shell cygpath -m $(goModCache))
lintCache := $(shell cygpath -m $(lintCache))
export GOROOT := $(shell cygpath -m $(GOROOT))
else
nativeGobin := $(gobin)
endif

ifneq ($(goCache),$(GOCACHE))
export GOCACHE := $(goCache)
endif
ifneq ($(goModCache),$(GOMODCACHE))
export GOMODCACHE := $(goModCache)
endif
ifneq ($(lintCache),$(GOLANGCI_LINT_CACHE))
export GOLANGCI_LINT_CACHE := $(lintCache)
endif

ifneq (,$(RACE_DETECTOR))
# cgo is required for using the race detector.
export CGO_ENABLED := 1
else
export CGO_ENABLED := 0
endif
# end go runtime settings

# Ensure the build directory exists, since most targets require it.
$(shell mkdir -p $(buildDir))

# start evergreen specific configuration
goos := $(GOOS)
goarch := $(GOARCH)
ifeq ($(goos),)
goos := $(shell $(gobin) env GOOS 2> /dev/null)
endif
ifeq ($(goarch),)
goarch := $(shell $(gobin) env GOARCH 2> /dev/null)
endif

clientBuildDir := clients
macOSPlatforms := darwin_amd64 $(if $(STAGING_ONLY),,darwin_arm64)
linuxPlatforms := linux_amd64 $(if $(STAGING_ONLY),,linux_s390x linux_arm64 linux_ppc64le)
windowsPlatforms := windows_amd64
unixBinaryBasename := evergreen
windowsBinaryBasename := evergreen.exe
macOSBinaries := $(foreach platform,$(macOSPlatforms),$(clientBuildDir)/$(platform)/$(unixBinaryBasename))
linuxBinaries := $(foreach platform,$(linuxPlatforms),$(clientBuildDir)/$(platform)/$(unixBinaryBasename))
windowsBinaries := $(foreach platform,$(windowsPlatforms),$(clientBuildDir)/$(platform)/$(windowsBinaryBasename))
clientBinaries := $(macOSBinaries) $(linuxBinaries) $(windowsBinaries)

clientSource := cmd/evergreen/evergreen.go
uiFiles := $(shell find public/static -not -path "./public/static/app" -name "*.js" -o -name "*.css" -o -name "*.html")

distArtifacts :=  ./public ./service/templates
distContents := $(clientBinaries) $(distArtifacts)
srcFiles := makefile $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -name "*_test.go" -not -path "./scripts/*" -not -path "*\#*")
testSrcFiles := makefile $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -path "*\#*")
currentHash := $(shell git rev-parse HEAD)
agentVersion := $(shell grep "AgentVersion" config.go | tr -d '\tAgentVersion = ' | tr -d '"')
ldFlags := $(if $(DEBUG_ENABLED),,-w -s )-X=github.com/evergreen-ci/evergreen.BuildRevision=$(currentHash)
gcFlags := $(if $(STAGING_ONLY),-N -l,)
karmaFlags := $(if $(KARMA_REPORTER),--reporters $(KARMA_REPORTER),)

goLintInstallerVersion := "v1.51.2"
goLintInstallerChecksum := "0e09dedc7e35f511b7924b885e50d7fe48eef25bec78c86f22f5b5abd24976cc"
# end evergreen specific configuration

######################################################################
##
## Build rules and instructions for building evergreen binaries and targets.
##
######################################################################


# start rules for building services and clients
ifeq ($(OS),Windows_NT)
localClientBinary := $(clientBuildDir)/$(goos)_$(goarch)/$(windowsBinaryBasename)
else
localClientBinary := $(clientBuildDir)/$(goos)_$(goarch)/$(unixBinaryBasename)
endif
cli:$(localClientBinary)
clis:$(clientBinaries)
$(clientBuildDir)/%/$(unixBinaryBasename) $(clientBuildDir)/%/$(windowsBinaryBasename):$(buildDir)/build-cross-compile $(srcFiles) go.mod go.sum
	./$(buildDir)/build-cross-compile -buildName=$* -ldflags="$(ldFlags)" -gcflags="$(gcFlags)" -goBinary="$(nativeGobin)" -directory=$(clientBuildDir) -source=$(clientSource) -output=$@
sign-macos:$(foreach platform,$(macOSPlatforms),$(clientBuildDir)/$(platform)/.signed)
# Targets to upload the CLI binaries to S3.
$(buildDir)/upload-s3:cmd/upload-s3/upload-s3.go
	@$(gobin) build -o $@ $<
upload-clis:$(buildDir)/upload-s3 clis
	$(buildDir)/upload-s3 -bucket="${BUCKET_NAME}" -local="${LOCAL_PATH}" -remote="${REMOTE_PATH}" -exclude="${EXCLUDE_PATTERN}"
phony += cli clis upload-clis sign-macos
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

# set-smoke-vars is necessary for the smoke test to run correctly. The AWS credentials are needed to run AWS-related
# commands such as s3.put. The agent revision must be set to the current version because the agent will have to exit
# under the expectation that it will be redeployed if it's outdated (and the smoke test cannot deploy agents).
set-smoke-vars:$(buildDir)/.load-smoke-data $(buildDir)/set-project-var $(buildDir)/set-var
	@$(buildDir)/set-project-var -dbName mci_smoke -key aws_key -value $(AWS_KEY)
	@$(buildDir)/set-project-var -dbName mci_smoke -key aws_secret -value $(AWS_SECRET)
	@$(buildDir)/set-var -dbName=mci_smoke -collection=hosts -id=localhost -key=agent_revision -value=$(agentVersion)
	@$(buildDir)/set-var -dbName=mci_smoke -collection=pods -id=localhost -key=agent_version -value=$(agentVersion)

# set-smoke-git-config is necessary for the smoke test to submit a manual patch because the patch command uses git
# metadata.
set-smoke-git-config:
	git config user.name username
	git config user.email email
load-smoke-data:$(buildDir)/.load-smoke-data
load-local-data:$(buildDir)/.load-local-data
$(buildDir)/.load-smoke-data:$(buildDir)/load-smoke-data
	./$<
	@touch $@
$(buildDir)/.load-local-data:$(buildDir)/load-smoke-data
	./$< -path testdata/local -dbName $(if $(DB_NAME),$(DB_NAME),evergreen_local) -amboyDBName amboy_local
	@touch $@
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

# start output files
testOutput := $(foreach target,$(packages) $(testOnlyPackages),$(buildDir)/output.$(target).test)
lintOutput := $(foreach target,$(packages) $(lintOnlyPackages),$(buildDir)/output.$(target).lint)
coverageOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage)
coverageHtmlOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage.html)
# end output files

curlRetryOpts := --retry 10 --retry-max-time 120

# lint setup targets
$(buildDir)/.lintSetup:$(buildDir)/golangci-lint
	@touch $@
$(buildDir)/golangci-lint:
	@curl $(curlRetryOpts) -o "$(buildDir)/install.sh" https://raw.githubusercontent.com/golangci/golangci-lint/$(goLintInstallerVersion)/install.sh
	@echo "$(goLintInstallerChecksum) $(buildDir)/install.sh" | sha256sum --check
	@bash $(buildDir)/install.sh -b $(buildDir) $(goLintInstallerVersion) && touch $@
$(buildDir)/run-linter:cmd/run-linter/run-linter.go $(buildDir)/.lintSetup
	$(gobin) build -ldflags "-w" -o $@ $<
# end lint setup targets

# generate lint JSON document for evergreen
generate-lint:$(buildDir)/generate-lint.json
$(buildDir)/generate-lint.json:$(buildDir)/generate-lint $(srcFiles)
	./$(buildDir)/generate-lint
$(buildDir)/generate-lint:cmd/generate-lint/generate-lint.go
	$(gobin) build -ldflags "-w" -o  $@ $<
# end generate lint

# parse a host.create file and set expansions
parse-host-file:$(buildDir)/parse-host-file
	./$(buildDir)/parse-host-file --file $(HOST_FILE)
$(buildDir)/parse-host-file:cmd/parse-host-file/parse-host-file.go
	$(gobin) build -o $@ $<
$(buildDir)/expansions.yml:$(buildDir)/parse-host-file
# end host.create file parsing

# npm setup
$(buildDir)/.npmSetup:
	cd $(nodeDir) && $(if $(NODE_BIN_PATH),export PATH=${PATH}:$(NODE_BIN_PATH) && ,)npm install
	touch $@
# end npm setup


# distribution targets and implementation
$(buildDir)/build-cross-compile:cmd/build-cross-compile/build-cross-compile.go makefile
	GOOS="" GOARCH="" $(gobin) build -o $@ $<
$(buildDir)/make-tarball:cmd/make-tarball/make-tarball.go
	GOOS="" GOARCH="" $(gobin) build -o $@ $<

$(buildDir)/sign-executable:cmd/sign-executable/sign-executable.go
	$(gobin) build -o $@ $<
$(buildDir)/macnotary:$(buildDir)/sign-executable
	./$< get-client --download-url $(NOTARY_CLIENT_URL) --destination $@
$(clientBuildDir)/%/.signed:$(buildDir)/sign-executable $(clientBuildDir)/%/$(unixBinaryBasename) $(buildDir)/macnotary
	./$< sign --client $(buildDir)/macnotary --executable $(@D)/$(unixBinaryBasename) --server-url $(NOTARY_SERVER_URL) --bundle-id $(EVERGREEN_BUNDLE_ID)
	touch $@

dist-staging:
	STAGING_ONLY=1 DEBUG_ENABLED=1 SIGN_MACOS= $(MAKE) dist
dist-unsigned:
	SIGN_MACOS= $(MAKE) dist
dist:$(buildDir)/dist.tar.gz
$(buildDir)/dist.tar.gz:$(buildDir)/make-tarball $(clientBinaries) $(uiFiles) $(if $(SIGN_MACOS),sign-macos)
	./$< --name $@ --prefix $(name) $(foreach item,$(distContents),--item $(item)) --exclude "public/node_modules" --exclude "clients/.cache"
# end main build

# userfacing targets for basic build and development operations
build:cli
lint:$(foreach target,$(packages) $(lintOnlyPackages),$(buildDir)/output.$(target).lint)
test:$(foreach target,$(packages) $(testOnlyPackages),test-$(target))
js-test:$(buildDir)/.npmSetup
	cd $(nodeDir) && $(if $(NODE_BIN_PATH),export PATH=${PATH}:$(NODE_BIN_PATH) && ,)./node_modules/.bin/karma start static/js/tests/conf/karma.conf.js $(karmaFlags)
coverage:$(coverageOutput)
coverage-html:$(coverageHtmlOutput)
list-tests:
	@echo -e "test targets:" $(foreach target,$(packages),\\n\\ttest-$(target))
phony += lint build test coverage coverage-html list-tests
.PRECIOUS:$(testOutput) $(lintOutput) $(coverageOutput) $(coverageHtmlOutput)
# end front-ends

# start module management targets
mod-tidy:
	$(gobin) mod tidy
verify-mod-tidy:
	$(gobin) run cmd/verify-mod-tidy/verify-mod-tidy.go -goBin="$(gobin)"
phony += mod-tidy verify-mod-tidy
# end module management targets

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
# end convenience targets


# start test and coverage artifacts
#    This varable includes everything that the tests actually need to
#    run. (The "build" target is intentional and makes these targetsb
#    rerun as expected.)
testRunDeps := $(name)
testArgs := -v
dlvArgs := -test.v
testRunEnv := EVGHOME=$(evghome)
ifeq (,$(GOCONVEY_REPORTER))
	testRunEnv += GOCONVEY_REPORTER=silent
endif
ifneq (,$(SETTINGS_OVERRIDE))
testRunEnv += SETTINGS_OVERRIDE=$(SETTINGS_OVERRIDE)
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
$(buildDir)/output.%.test: .FORCE
	$(testRunEnv) $(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) 2>&1 | tee $@
# test-agent-command is special because it requires that the Evergreen binary be compiled to run some of the tests.
$(buildDir)/output.agent-command.test: cli .FORCE
	$(testRunEnv) $(gobin) test $(testArgs) ./agent/command 2>&1 | tee $@
# Smoke tests are special because they require that the Evergreen binary is compiled and the smoke test data is loaded.
$(buildDir)/output.smoke-internal-%.test: cli load-smoke-data
	$(testRunEnv) $(gobin) test $(testArgs) ./smoke/internal/$(if $(subst $(name),,$*),$(subst -,/,$*),) 2>&1 | tee $@
$(buildDir)/output-dlv.%.test: .FORCE
	$(testRunEnv) dlv test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) -- $(dlvArgs) 2>&1 | tee $@
$(buildDir)/output.%.coverage: .FORCE
	$(testRunEnv) $(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) -covermode=count -coverprofile $@ | tee $(buildDir)/output.$*.test
	@-[ -f $@ ] && $(gobin) tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
#  targets to generate gotest output from the linter.
ifneq (go,$(gobin))
# We have to handle the PATH specially for linting in CI, because if the PATH has a different version of the Go
# binary in it, the linter won't work properly.
lintEnvVars := PATH="$(shell dirname $(gobin)):$(PATH)"
endif
$(buildDir)/output.%.lint: $(buildDir)/run-linter .FORCE
	@$(lintEnvVars) ./$< --output=$@ --lintBin=$(buildDir)/golangci-lint -customLinters="$(gobin) run github.com/evergreen-ci/evg-lint/evg-lint -set_exit_status" --lintArgs="--timeout=5m" --packages='$*'
$(buildDir)/output.%.coverage.html:$(buildDir)/output.%.coverage
	$(gobin) tool cover -html=$< -o $@
# end test and coverage artifacts

clean-lobster:
	rm -rf $(lobsterTempDir)
phony += clean-lobster

update-lobster: clean-lobster
	EVGHOME=$(evghome) LOBSTER_TEMP_DIR=$(lobsterTempDir) scripts/update-lobster.sh

# clean and other utility targets
clean: clean-lobster
	rm -rf $(buildDir) $(clientBuildDir)
phony += clean

gqlgen:
	$(gobin) run github.com/99designs/gqlgen generate

swaggo: 
	$(MAKE) swaggo-format swaggo-build swaggo-render

swaggo-install:
	$(gobin) install github.com/swaggo/swag/cmd/swag@latest

swaggo-format:
	swag fmt -g service/service.go

swaggo-build:
	swag init -g service/service.go -o $(buildDir) --outputTypes json

swaggo-render:
	npx @redocly/cli build-docs $(buildDir)/swagger.json -o $(buildDir)/redoc-static.html

phony += swaggo swaggo-install swaggo-format swaggo-build swaggo-render

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
	cd mongodb && curl $(curlRetryOpts) "$(MONGODB_URL)" -o mongodb.tgz && $(MONGODB_DECOMPRESS) mongodb.tgz && chmod +x ./mongodb-*/bin/*
	cd mongodb && mv ./mongodb-*/bin/* . && rm -rf db_files && rm -rf db_logs && mkdir -p db_files && mkdir -p db_logs
mongodb/.get-mongosh:
	rm -rf mongosh
	mkdir -p mongosh
	cd mongosh && curl $(curlRetryOpts) "$(MONGOSH_URL)" -o mongosh.tgz && $(MONGOSH_DECOMPRESS) mongosh.tgz && chmod +x ./mongosh-*/bin/*
	cd mongosh && mv ./mongosh-*/bin/* .
get-mongodb:mongodb/.get-mongodb
	@touch $<
get-mongosh: mongodb/.get-mongosh
	@touch $<
start-mongod:mongodb/.get-mongodb
ifdef AUTH_ENABLED
	echo "replica set key" > ./mongodb/keyfile.txt
	chmod 600 ./mongodb/keyfile.txt
endif
	./mongodb/mongod $(if $(AUTH_ENABLED),--auth --keyFile ./mongodb/keyfile.txt,) --dbpath ./mongodb/db_files --port 27017 --replSet evg --oplogSize 10
configure-mongod:mongodb/.get-mongodb mongodb/.get-mongosh
	./mongosh/mongosh --nodb ./cmd/init-mongo/wait_for_mongo.js
	@echo "mongod is up"
	./mongosh/mongosh --eval 'rs.initiate()'
ifdef AUTH_ENABLED
	./mongosh/mongosh ./cmd/init-mongo/create_auth_user.js
endif
	@echo "configured mongod"
# end mongodb targets


# configure special (and) phony targets
.FORCE:
.PHONY:$(phony) .FORCE
.DEFAULT_GOAL := build
