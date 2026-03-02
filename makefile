# start project configuration
name := evergreen
buildDir := bin
nodeDir := public
packages := $(name) agent agent-command agent-executor agent-globals agent-util agent-taskexec agent-internal agent-internal-client agent-internal-redactor agent-internal-taskoutput agent-internal-testutil operations cloud cloud-userdata
packages += db util units graphql thirdparty thirdparty-docker auth scheduler model validator service repotracker mock
packages += model-annotations model-patch model-artifact model-host model-build model-event model-task model-user model-distro model-manifest model-testresult model-log model-testlog model-parsley
packages += model-commitqueue model-cache model-githubapp model-hoststat model-cost model-s3lifecycle
packages += rest-client rest-data rest-route rest-model trigger model-alertrecord model-notification model-taskstats model-reliability
packages += taskoutput cloud-parameterstore cloud-parameterstore-fakeparameter
lintOnlyPackages := api apimodels testutil model-manifest model-testutil model-testresult-testutil service-testutil service-graphql db-mgo db-mgo-bson db-mgo-internal-json rest
lintOnlyPackages += smoke-internal smoke-internal-host smoke-internal-agentmonitor smoke-internal-endpoint thirdparty-clients-fws
testOnlyPackages := service-graphql smoke-internal-host smoke-internal-agentmonitor smoke-internal-endpoint # has only test files so can't undergo all operations
orgName := evergreen-ci
orgPath := github.com/$(orgName)
projectPath := $(orgPath)/$(name)
evghome := $(abspath .)
ifeq ($(OS),Windows_NT)
	evghome := $(shell cygpath -m $(evghome))
endif
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
macOSPlatforms := $(if $(STAGING_ONLY),,darwin_amd64 darwin_arm64)
linuxPlatforms := linux_amd64 $(if $(STAGING_ONLY),,linux_s390x linux_arm64 linux_ppc64le)
windowsPlatforms := windows_amd64
unixBinaryBasename := evergreen
windowsBinaryBasename := evergreen.exe
macOSBinaries := $(foreach platform,$(macOSPlatforms),$(clientBuildDir)/$(platform)/$(unixBinaryBasename))
linuxBinaries := $(foreach platform,$(linuxPlatforms),$(clientBuildDir)/$(platform)/$(unixBinaryBasename))
windowsBinaries := $(foreach platform,$(windowsPlatforms),$(clientBuildDir)/$(platform)/$(windowsBinaryBasename))
clientBinaries := $(macOSBinaries) $(linuxBinaries) $(windowsBinaries)

clientSource := cmd/evergreen/evergreen.go

currentHash := $(shell git rev-parse HEAD)
agentVersion := $(shell grep "AgentVersion" config.go | tr -d '\tAgentVersion = ' | tr -d '"')
ldFlags := $(if $(DEBUG_ENABLED),,-w -s )-X=github.com/evergreen-ci/evergreen.BuildRevision=$(currentHash)
gcFlags := $(if $(STAGING_ONLY),-N -l,)
karmaFlags := $(if $(KARMA_REPORTER),--reporters $(KARMA_REPORTER),)

golangciLintVersion := "v1.64.5"
golangciLintInstallerChecksum := "9e99d38f3213411a1b6175e5b535c72e37c7ed42ccf251d331385a3f97b695e7"
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
$(clientBuildDir)/%/$(unixBinaryBasename) $(clientBuildDir)/%/$(windowsBinaryBasename):$(buildDir)/build-cross-compile .FORCE
	./$(buildDir)/build-cross-compile -buildName=$* -ldflags="$(ldFlags)" -gcflags="$(gcFlags)" -goBinary="$(nativeGobin)" -directory=$(clientBuildDir) -source=$(clientSource) -output=$@

build-linux_%: $(clientBuildDir)/linux_%/$(unixBinaryBasename);
build-windows_%: $(clientBuildDir)/windows_%/$(windowsBinaryBasename);
build-darwin_%: $(clientBuildDir)/darwin_%/$(unixBinaryBasename) $(clientBuildDir)/darwin_%/.signed;

build-linux-staging_%: $(clientBuildDir)/linux_%/$(unixBinaryBasename);
build-windows-staging_%: $(clientBuildDir)/windows_%/$(windowsBinaryBasename);

build-darwin-unsigned_%: $(clientBuildDir)/darwin_%/$(unixBinaryBasename);

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

# set-smoke-vars is necessary for the smoke test to run correctly. The AWS credentials are needed to run AWS-related
# commands such as s3.put. The agent revision must be set to the current version because the agent will have to exit
# under the expectation that it will be redeployed if it's outdated (and the smoke test cannot deploy agents).
set-smoke-vars:$(buildDir)/.load-smoke-data $(buildDir)/set-project-var $(buildDir)/set-var
	@$(buildDir)/set-project-var -dbName mci_smoke -key aws_key -value $(AWS_ACCESS_KEY_ID)
	@$(buildDir)/set-project-var -dbName mci_smoke -key aws_secret -value $(AWS_SECRET_ACCESS_KEY)
	@$(buildDir)/set-project-var -dbName mci_smoke -key aws_token -value $(AWS_SESSION_TOKEN)
	@$(buildDir)/set-var -dbName=mci_smoke -collection=hosts -id=localhost -key=agent_revision -value=$(agentVersion)

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
## Build and Test targets and mechisms.
##
######################################################################

# most of the targets and variables in this section are generic
# instructions for go programs of all kinds, and are not particularly
# specific to evergreen; though the build targets are more specific than the rest.

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
	@curl $(curlRetryOpts) -o "$(buildDir)/install.sh" https://raw.githubusercontent.com/golangci/golangci-lint/$(golangciLintVersion)/install.sh
	@echo "$(golangciLintInstallerChecksum) *$(buildDir)/install.sh" | shasum --check
	@bash $(buildDir)/install.sh -b $(buildDir) $(golangciLintVersion) && touch $@
$(buildDir)/run-linter:cmd/run-linter/run-linter.go $(buildDir)/.lintSetup
	$(gobin) build -ldflags "-w" -o $@ $<
# end lint setup targets

# generate lint JSON document for evergreen
generate-lint:$(buildDir)/generate-lint.json
$(buildDir)/generate-lint.json:$(buildDir)/generate-lint .FORCE
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

# distribution targets and implementation
$(buildDir)/build-cross-compile:cmd/build-cross-compile/build-cross-compile.go makefile
	GOOS="" GOARCH="" $(gobin) build -o $@ $<

$(buildDir)/sign-executable:cmd/sign-executable/sign-executable.go
	$(gobin) build -o $@ $<
$(buildDir)/macnotary:$(buildDir)/sign-executable
	./$< get-client --download-url $(NOTARY_CLIENT_URL) --destination $@
$(clientBuildDir)/%/.signed:$(buildDir)/sign-executable $(clientBuildDir)/%/$(unixBinaryBasename) $(buildDir)/macnotary
	./$< sign --client $(buildDir)/macnotary --executable $(@D)/$(unixBinaryBasename) --server-url $(NOTARY_SERVER_URL) --bundle-id $(EVERGREEN_BUNDLE_ID)
	touch $@
# end main build

# userfacing targets for basic build and development operations
build:cli
lint:$(foreach target,$(packages) $(lintOnlyPackages),$(buildDir)/output.$(target).lint)
test:$(foreach target,$(packages) $(testOnlyPackages),test-$(target))
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
testArgs += -ldflags="$(ldFlags) -X=github.com/evergreen-ci/evergreen/testutil.ExecutionEnvironmentType=test -X=github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter.ExecutionEnvironmentType=test"
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


gqlgen:
	$(gobin) tool github.com/99designs/gqlgen generate
	$(gobin) run cmd/gqlgen/generate_secret_fields.go

govul-install:
	$(gobin) install golang.org/x/vuln/cmd/govulncheck@latest

swaggo:
	$(MAKE) swaggo-format swaggo-build swaggo-render

swaggo-install:
	$(gobin) install github.com/swaggo/swag/cmd/swag@latest

swaggo-format:
	swag fmt -g service/service.go

swaggo-build:
	swag init -g service/service.go -o $(buildDir) --outputTypes json --parseDependency --parseInternal

swaggo-render:
	npx @redocly/cli build-docs $(buildDir)/swagger.json -o $(buildDir)/redoc-static.html


# Variables
OPENAPI_FWS_CONFIG_URL := https://foliage-web-services.cloud-build.prod.corp.mongodb.com/foliage_web_services.json
OPENAPI_FWS_SCHEMA := $(buildDir)/foliage_web_services.json
OPENAPI_FWS_OUTPUT_DIR := thirdparty/clients/fws
OPENAPI_FWS_CONFIG := packageName=fws,packageVersion=1.0.0,packageTitle=FoliageWebServices,packageDescription="Foliage Web Services",apiTests=false,modelTests=false
OPENAPI_GENERATOR := bin/openapi-generator-cli.sh

# Main rule for generating the client
fws-client: download-fws-config generate-fws-client

download-fws-config:
	@echo "Authenticating to Kanopy..." && \
	KANOPY_KEY=$$(kanopy-oidc login) && \
	echo "Downloading OpenAPI config..." && \
	curl -H "Authorization: Bearer $$KANOPY_KEY" -L -o $(OPENAPI_FWS_SCHEMA) $(OPENAPI_FWS_CONFIG_URL) && \
	echo "Downloaded OpenAPI config"

generate-fws-client:
	@echo "Generating OpenAPI client..." && \
	scripts/setup-openapi-client.sh $(OPENAPI_FWS_SCHEMA) $(OPENAPI_FWS_OUTPUT_DIR) $(OPENAPI_GENERATOR) $(OPENAPI_FWS_CONFIG) && \
	echo "Generating OpenAPI client done." && \
	echo "Swaggo format..." && \
	make swaggo-format && \
	echo "Swaggo format done."


phony += swaggo swaggo-install swaggo-format swaggo-build swaggo-render fws-client generate-fws-client download-fws-config

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
	./mongodb/mongod --dbpath ./mongodb/db_files --port 27017 --replSet evg --oplogSize 10
configure-mongod:mongodb/.get-mongodb mongodb/.get-mongosh
	./mongosh/mongosh --nodb ./cmd/init-mongo/wait_for_mongo.js
	@echo "mongod is up"
	./mongosh/mongosh --eval 'rs.initiate()'
	@echo "configured mongod"
# end mongodb targets

# Installs a newer version of git from source than most distros have available.
install-git:
	curl $(curlRetryOpts) -LO https://github.com/git/git/archive/refs/tags/v2.52.0.tar.gz
	tar -xzf v2.52.0.tar.gz
	cd git-2.52.0 && $(MAKE) configure && ./configure --prefix=$(CURDIR)/$(buildDir)/git && $(MAKE) all && $(MAKE) install
phony += install-git

# configure special (and) phony targets
.FORCE:
.PHONY:$(phony) .FORCE
.DEFAULT_GOAL := build
