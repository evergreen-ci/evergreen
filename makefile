# start project configuration
name := evergreen
buildDir := bin
packages := $(name) agent db cli remote subprocess taskrunner util plugin hostinit
packages += command plugin-builtin-attach plugin-builtin-manifest plugin-builtin-buildbaron
packages += notify thirdparty alerts auth scheduler model hostutil validator service monitor repotracker
packages += model-patch model-artifact model-host model-build model-event model-task
packages += cloud-providers cloud-providers-docker cloud-providers-ec2 cloud-providers-gce cloud-providers-openstack cloud-providers-vsphere
packages += rest-client rest-data rest-route rest-model migrations
orgPath := github.com/evergreen-ci
projectPath := $(orgPath)/$(name)
# end project configuration


# start evergreen specific configuration
unixPlatforms := linux_amd64 darwin_amd64 $(if $(STAGING_ONLY),,linux_386 linux_s390x linux_arm64 linux_ppc64le solaris_amd64)
windowsPlatforms := windows_amd64 $(if $(STAGING_ONLY),,windows_386)

goos := $(shell go env GOOS)
goarch := $(shell go env GOARCH)
gobin := $(shell which go)

clientBuildDir := clients

clientBinaries := $(foreach platform,$(unixPlatforms) $(if $(STAGING_ONLY),,freebsd_amd64),$(clientBuildDir)/$(platform)/evergreen)
clientBinaries += $(foreach platform,$(windowsPlatforms),$(clientBuildDir)/$(platform)/evergreen.exe)

clientSource := cli/main/cli.go

distArtifacts :=  ./public ./service/templates ./service/plugins ./alerts/templates ./notify/templates
distContents := $(clientBuildDir) $(distArtifacts)
distTestContents := $(foreach pkg,$(packages),$(buildDir)/test.$(pkg))
distTestRaceContents := $(foreach pkg,$(packages),$(buildDir)/race.$(pkg))
srcFiles := makefile $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -name "*_test.go" -not -path "./scripts/*" -not -path "*\#*")
testSrcFiles := makefile $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -path "*\#*")
currentHash := $(shell git rev-parse HEAD)
ldFlags := "-w -s -X=github.com/evergreen-ci/evergreen.BuildRevision=$(currentHash)"
# end evergreen specific configuration


# start linting configuration
#   package, testing, and linter dependencies specified
#   separately. This is a temporary solution: eventually we should
#   vendorize all of these dependencies.
lintDeps := github.com/alecthomas/gometalinter
#   include test files and give linters 40s to run to avoid timeouts
lintArgs := --tests --deadline=5m --vendor --aggregate --sort=line
lintArgs += --vendored-linters --enable-gc
#   gotype produces false positives because it reads .a files which
#   are rarely up to date.
lintArgs += --disable="gotype" --disable="gas" --disable="gocyclo"
lintArgs += --disable="golint" --disable="goconst" --disable="dupl"
lintArgs += --disable="varcheck" --disable="structcheck" --disable="aligncheck"
lintArgs += --skip="$(buildDir)" --skip="scripts" --skip="$(gopath)"
#  add and configure additional linters
lintArgs += --enable="misspell" # --enable="lll" --line-length=100
#  suppress some lint errors (logging methods could return errors, and error checking in defers.)
lintArgs += --exclude=".*([mM]ock.*ator|modadvapi32|osSUSE) is unused \((deadcode|unused|megacheck)\)$$"
lintArgs += --exclude="error return value not checked \(defer .* \(errcheck\)$$"
# end lint configuration


######################################################################
##
## Build rules and instructions for building evergreen binaries and targets.
##
######################################################################


# start rules for services and clients
plugins:$(buildDir)/.plugins
$(buildDir)/.plugins:Plugins install_plugins.sh
	./install_plugins.sh
	@touch $@
# end rules for building server binaries


# start rules for building services and clients
ifeq ($(OS),Windows_NT)
cli:$(clientBuildDir)/$(goos)_$(goarch)/evergreen.exe
else
cli:$(clientBuildDir)/$(goos)_$(goarch)/evergreen
endif
clis:$(clientBinaries)
$(clientBuildDir)/%/evergreen $(clientBuildDir)/%/evergreen.exe:$(buildDir)/build-cross-compile $(srcFiles)
	@./$(buildDir)/build-cross-compile -buildName=$* -ldflags=$(ldFlags) -goBinary="$(gobin)" $(if $(RACE_ENABLED),-race ,)-directory=$(clientBuildDir) -source=$(clientSource) -output=$@
$(clientBuildDir)/version:
	@mkdir -p $(dir $@)
	git rev-parse HEAD >| $@
phony += $(clientBuildDir)/version cli clis
$(buildDir)/test.service $(buildDir)/race.service:$(clientBuildDir)/version
$(buildDir)/test.agent $(buildDir)/race.agent:$(clientBuildDir)/version
$(buildDir)/test.proto $(buildDir)/race.proto:$(clientBuildDir)/version
# end client build directives


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
gopath := $(shell go env GOPATH)
ifeq ($(OS),Windows_NT)
gopath := $(shell cygpath -m $(gopath))
endif
testOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).test)
raceOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).race)
testBin := $(foreach target,$(packages),$(buildDir)/test.$(target))
raceBin := $(foreach target,$(packages),$(buildDir)/race.$(target))
lintTargets := $(foreach target,$(packages),lint-$(target))
coverageOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage)
coverageHtmlOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage.html)
$(gopath)/src/%:
	@-[ ! -d $(gopath) ] && mkdir -p $(gopath) || true
	go get $(subst $(gopath)/src/,,$@)
# end dependency installation tools


# lint setup targets
lintDeps := $(addprefix $(gopath)/src/,$(lintDeps))
$(buildDir)/.lintSetup:$(lintDeps)
	@mkdir -p $(buildDir)
	$(gopath)/bin/gometalinter --update --force --install >/dev/null && touch $@
$(buildDir)/run-linter:scripts/run-linter.go $(buildDir)/.lintSetup
	go build -o $@ $<
# end lint setup targets


# distribution targets and implementation
$(buildDir)/build-cross-compile:scripts/build-cross-compile.go makefile
	@mkdir -p $(buildDir)
	@GOOS="" GOARCH="" go build -o $@ $<
	@echo go build -o $@ $<
$(buildDir)/make-tarball:scripts/make-tarball.go
	@mkdir -p $(buildDir)
	@GOOS="" GOARCH="" go build -o $@ $<
	@echo go build -o $@ $<
dist:$(buildDir)/dist.tar.gz
dist-test:$(buildDir)/dist-test.tar.gz
dist-source:$(buildDir)/dist-source.tar.gz
$(buildDir)/dist.tar.gz:$(buildDir)/make-tarball plugins clis $(clientBuildDir)/version
	./$< --name $@ --prefix $(name) $(foreach item,$(distContents),--item $(item))
$(buildDir)/dist-test.tar.gz:$(buildDir)/make-tarball plugins makefile $(distTestContents) # $(distTestRaceContents)
	./$< -name $@ --prefix $(name)-tests $(foreach item,$(distContents) $(distTestContents),--item $(item)) $(foreach item,,--item $(item))
$(buildDir)/dist-source.tar.gz:$(buildDir)/make-tarball $(srcFiles) $(testSrcFiles) makefile
	./$< --name $@ --prefix $(name) $(subst $(name),,$(foreach pkg,$(packages),--item ./$(subst -,/,$(pkg)))) --item ./scripts --item makefile --exclude "$(name)" --exclude "^.git/" --exclude "$(buildDir)/"
# end main build


# userfacing targets for basic build and development operations
build:cli
lint:$(buildDir)/output.lint
test:$(foreach target,$(packages),test-$(target))
race:$(foreach target,$(packages),race-$(target))
coverage:$(coverageOutput)
coverage-html:$(coverageHtmlOutput)
list-tests:
	@echo -e "test targets:" $(foreach target,$(packages),\\n\\ttest-$(target))
list-race:
	@echo -e "test (race detector) targets:" $(foreach target,$(packages),\\n\\trace-$(target))
phony += lint lint-deps build build-race race test coverage coverage-html list-race list-tests
.PRECIOUS:$(testOutput) $(raceOutput) $(coverageOutput) $(coverageHtmlOutput)
.PRECIOUS:$(foreach target,$(packages),$(buildDir)/test.$(target))
.PRECIOUS:$(foreach target,$(packages),$(buildDir)/race.$(target))
.PRECIOUS:$(foreach target,$(packages),$(buildDir)/output.$(target).lint)
.PRECIOUS:$(buildDir)/output.lint
# end front-ends


# convenience targets for runing tests and coverage tasks on a
# specific package.
race-%:$(buildDir)/output.%.race
	@grep -s -q -e "^PASS" $< && ! grep -s -q "^WARNING: DATA RACE" $<
test-%:$(buildDir)/output.%.test
	@grep -s -q -e "^PASS" $<
coverage-%:$(buildDir)/output.%.coverage
	@grep -s -q -e "^PASS" $(subst coverage,test,$<)
html-coverage-%:$(buildDir)/output.%.coverage $(buildDir)/output.%.coverage.html
	@grep -s -q -e "^PASS" $(subst coverage,test,$<)
lint-%:$(buildDir)/output.%.lint
	@grep -v -s -q "^--- FAIL" $<
# end convienence targets


# start vendoring configuration
vendor-clean:
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/mongodb/grip/vendor/golang.org/x/oauth2/
	rm -rf vendor/github.com/mongodb/amboy/vendor/gopkg.in/mgo.v2/
	rm -rf vendor/github.com/mongodb/amboy/vendor/gopkg.in/yaml.v2/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/tychoish/gimlet/vendor/github.com/gorilla/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/tychoish/gimlet/vendor/github.com/tylerb/graceful/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/tychoish/gimlet/vendor/github.com/urfave/negroni/
	rm -rf vendor/github.com/docker/docker/vendor/github.com/docker/go-connections/
	rm -rf vendor/github.com/docker/docker/vendor/github.com/Microsoft/go-winio/
phony += vendor-clean
# end vendoring tooling configuration


# start test and coverage artifacts
#    This varable includes everything that the tests actually need to
#    run. (The "build" target is intentional and makes these targetsb
#    rerun as expected.)
testRunDeps := $(name)
testTimeout := --test.timeout=10m
testArgs := -test.v $(testTimeout)
testRunEnv := EVGHOME=$(shell pwd)
ifeq ($(OS),Windows_NT)
testRunEnv := EVGHOME=$(shell cygpath -m `pwd`)
endif
ifneq (,$(SETTINGS_OVERRIDE))
testRunEnv += SETTINGS_OVERRIDE=$(SETTINGS_OVERRIDE)
endif
ifneq (,$(RUN_TEST))
testArgs += -test.run='$(RUN_TEST)'
endif
ifneq (,$(RUN_CASE))
testArgs += -testify.m='$(RUN_CASE)'
endif
ifneq (,$(RUN_COUNT))
testArgs += -test.count='$(RUN_COUNT)'
endif
#  targets to compile
$(buildDir)/test.%:$(testSrcFiles)
	go test -ldflags=$(ldFlags) $(if $(DISABLE_COVERAGE),,-covermode=count )-c -o $@ ./$(subst -,/,$*)
$(buildDir)/race.%:$(testSrcFiles)
	go test -ldflags=$(ldFlags) -race -c -o $@ ./$(subst -,/,$*)
#  targets to run any tests in the top-level package
$(buildDir)/test.$(name):$(testSrcFiles)
	go test -ldflags=$(ldFlags) $(if $(DISABLE_COVERAGE),,-covermode=count )-c -o $@ ./
$(buildDir)/race.$(name):$(testSrcFiles)
	go test -ldflags=$(ldFlags) -race -c -o $@ ./
#  targets to run the tests and report the output
$(buildDir)/output.%.test:$(buildDir)/test.% .FORCE
	$(testRunEnv) ./$< $(testArgs) 2>&1 | tee $@
$(buildDir)/output.%.race:$(buildDir)/race.% .FORCE
	$(testRunEnv) ./$< $(testArgs) 2>&1 | tee $@
#  targets to generate gotest output from the linter.
$(buildDir)/output.%.lint:$(buildDir)/run-linter $(testSrcFiles) .FORCE
	@./$< --output=$@ --lintArgs='$(lintArgs)' --packages='$*'
$(buildDir)/output.lint:$(buildDir)/run-linter .FORCE
	@./$< --output="$@" --lintArgs='$(lintArgs)' --packages="$(packages)"
#  targets to process and generate coverage reports
$(buildDir)/output.%.coverage:$(buildDir)/test.% .FORCE
	$(testRunEnv) ./$< $(testArgs) -test.coverprofile=./$@ 2>&1 | tee $(subst coverage,test,$@)
	@-[ -f $@ ] && go tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/output.%.coverage.html:$(buildDir)/output.%.coverage
	go tool cover -html=$< -o $@
# end test and coverage artifacts


# clean and other utility targets
clean:
	rm -rf $(lintDeps) $(buildDir)/test.* $(buildDir)/coverage.* $(buildDir)/race.* $(clientBuildDir)
phony += clean
# end dependency targets

# configure phony targets
.FORCE:
.PHONY:$(phony) .FORCE
