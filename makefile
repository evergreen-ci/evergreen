# start project configuration
name := evergreen
buildDir := bin
packages := $(name) agent db cli archive remote command taskrunner util plugin hostinit plugin-builtin-git
packages += plugin-builtin-gotest plugin-builtin-attach plugin-builtin-manifest plugin-builtin-archive
packages += plugin-builtin-shell plugin-builtin-s3copy plugin-builtin-expansions plugin-builtin-s3
packages += notify thirdparty alerts auth scheduler model hostutil validator service monitor repotracker
packages += model-patch model-artifact model-host model-build model-event model-task db-bsonutil
packages += plugin-builtin-attach-xunit cloud-providers cloud-providers-ec2 agent-comm
packages += rest-data rest-route rest-model
orgPath := github.com/evergreen-ci
projectPath := $(orgPath)/$(name)
# end project configuration


# start evergreen specific configuration
unixPlatforms := linux_amd64 linux_386 linux_s390x linux_arm64 linux_ppc64le solaris_amd64 darwin_amd64
windowsPlatforms := windows_amd64 windows_386
goos := $(shell go env GOOS)
goarch := $(shell go env GOARCH)

agentBuildDir := executables
clientBuildDir := clients

agentBinaries := $(foreach platform,$(unixPlatforms),$(agentBuildDir)/$(platform)/main)
agentBinaries += $(foreach platform,$(windowsPlatforms),$(agentBuildDir)/$(platform)/main.exe)
clientBinaries := $(foreach platform,$(unixPlatforms) freebsd_amd64,$(clientBuildDir)/$(platform)/evergreen)
clientBinaries += $(foreach platform,$(windowsPlatforms),$(clientBuildDir)/$(platform)/evergreen.exe)

binaries := $(buildDir)/evergreen_ui_server $(buildDir)/evergreen_runner $(buildDir)/evergreen_api_server
raceBinaries := $(foreach bin,$(binaries),$(bin).race)

agentSource := agent/main/agent.go
clientSource := cli/main/cli.go

distArtifacts :=  ./public ./service/templates ./service/plugins ./alerts/templates
distContents := $(agentBuildDir) $(clientBuildDir) $(distArtifacts)
distTestContents := $(foreach pkg,$(packages),$(buildDir)/test.$(pkg) $(buildDir)/race.$(pkg))

srcFiles := makefile $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -name "*_test.go" -not -path "./scripts/*" )
testSrcFiles := makefile $(shell find . -name "*.go" -not -path "./$(buildDir)/*")

projectCleanFiles := $(agentBuildDir) $(clientBuildDir)
# static rules for rule for building artifacts
define crossCompile
	@$(vendorGopath) ./$(buildDir)/build-cross-compile -buildName=$* -ldflags="-X=github.com/evergreen-ci/evergreen.BuildRevision=`git rev-parse HEAD`" -goBinary="`which go`" -output=$@
endef
# end evergreen specific configuration


# start linting configuration
#   package, testing, and linter dependencies specified
#   separately. This is a temporary solution: eventually we should
#   vendorize all of these dependencies.
lintDeps := github.com/alecthomas/gometalinter
#   include test files and give linters 40s to run to avoid timeouts
lintArgs := --tests --deadline=20m --vendor --aggregate --sort="line"
#   gotype produces false positives because it reads .a files which
#   are rarely up to date.
lintArgs += --disable="gotype" --disable="gas" --disable="gocyclo"
lintArgs += --disable="aligncheck" --disable="golint" --disable="goconst"
lintArgs += --skip="$(buildDir)" --skip="scripts" --skip="$(gopath)"
#  add and configure additional linters
lintArgs += --enable="goimports" --enable="misspell" --enable="unused" --enable="vet" --enable="unparam"
# lintArgs += --enable="lll" --line-length=100
lintArgs += --dupl-threshold=175
#  two similar functions triggered the duplicate warning, but they're not.
# lintArgs += --exclude="file is not goimported" # test files aren't imported
#  golint doesn't handle splitting package comments between multiple files.
# lintArgs += --exclude="package comment should be of the form \"Package .* \(golint\)"
#  suppress some lint errors (logging methods could return errors, and error checking in defers.)
lintArgs += --exclude=".*([mM]ock.*ator|modadvapi32|osSUSE) is unused \((deadcode|unused)\)$$"
lintArgs += --exclude=".*(procInfo|sysInfo|metricsCollector\).start|testSorter).*is unused.*\(unused|deadcode\)$$"
lintArgs += --exclude="error return value not checked \(defer.* \(errcheck\)$$"
lintArgs += --exclude="defers in this range loop.* \(staticcheck\)$$"
lintArgs += --exclude=".*should use time.Until instead of t.Sub\(time.Now\(\)\).* \(gosimple\)$$"
lintArgs += --exclude="suspect or:.*\(vet\)$$"
# end lint configuration


######################################################################
##
## Build rules and instructions for building evergreen binaries and targets.
##
######################################################################


# start rules for binding agents
#   build the server binaries:
plugins:$(buildDir)/.plugins
$(buildDir)/.plugins:Plugins install_plugins.sh
	./install_plugins.sh
	@touch $@
evergreen_api_server:$(buildDir)/evergreen_api_server
$(buildDir)/evergreen_api_server:service/api_main/apiserver.go $(buildDir)/build-cross-compile $(srcFiles)
	$(crossCompile) -directory=$(buildDir) -source=$<
evergreen_ui_server:$(buildDir)/evergreen_ui_server
$(buildDir)/evergreen_ui_server:service/ui_main/ui.go $(buildDir)/build-cross-compile $(srcFiles)
	$(crossCompile) -directory=$(buildDir) -source=$<
evergreen_runner:$(buildDir)/evergreen_runner
$(buildDir)/evergreen_runner:runner/main/runner.go $(buildDir)/build-cross-compile $(srcFiles)
	$(crossCompile) -directory=$(buildDir) -source=$<
#   build the server binaries with the race detector:
$(buildDir)/evergreen_api_server.race:service/api_main/apiserver.go $(buildDir)/build-cross-compile $(srcFiles)
	$(crossCompile) -race -directory=$(buildDir) -source=$<
$(buildDir)/evergreen_runner.race:runner/main/runner.go $(buildDir)/build-cross-compile $(srcFiles)
	$(crossCompile) -race -directory=$(buildDir) -source=$<
$(buildDir)/evergreen_ui_server.race:service/ui_main/ui.go $(buildDir)/build-cross-compile $(srcFiles)
	$(crossCompile) -race -directory=$(buildDir) -source=$<
phony += $(binaries) $(raceBinaries)
# end rules for building server binaries


# start rules for building agents and clis
ifeq ($(OS),Windows_NT)
agent:$(agentBuildDir)/$(goos)_$(goarch)/main.exe
cli:$(clientBuildDir)/$(goos)_$(goarch)/evergreen.exe
else
agent:$(agentBuildDir)/$(goos)_$(goarch)/main
cli:$(clientBuildDir)/$(goos)_$(goarch)/evergreen
endif
agents:$(agentBinaries)
clis:$(clientBinaries)
$(agentBuildDir)/%/main $(agentBuildDir)/%/main.exe:$(buildDir)/build-cross-compile $(agentBuildDir)/version $(srcFiles)
	$(crossCompile) -directory=$(agentBuildDir) -source=$(agentSource)
$(clientBuildDir)/%/evergreen $(clientBuildDir)/%/evergreen.exe:$(buildDir)/build-cross-compile $(srcFiles)
	$(crossCompile) -directory=$(clientBuildDir) -source=$(clientSource) -output=$@
$(agentBuildDir)/version:
	@mkdir -p $(dir $@)
	git rev-parse HEAD >| $@
phony += agents agent $(agentBuildDir)/version cli clis
# end agent build directives


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
	@-$(gopath)/bin/gometalinter --install >/dev/null && touch $@
$(buildDir)/run-linter:scripts/run-linter.go $(buildDir)/.lintSetup
	$(vendorGopath) go build -o $@ $<
# end lint setup targets


# distribution targets and implementation
$(buildDir)/build-cross-compile:scripts/build-cross-compile.go makefile
	@mkdir -p $(buildDir)
	@GOOS="" GOOARCH="" go build -o $@ $<
	@echo go build -o $@ $<
$(buildDir)/make-tarball:scripts/make-tarball.go $(buildDir)/render-gopath
	$(vendorGopath) go build -o $@ $<
dist:$(buildDir)/dist.tar.gz
dist-test:$(buildDir)/dist-test.tar.gz
dist-race: $(buildDir)/dist-race.tar.gz
dist-source:$(buildDir)/dist-source.tar.gz
$(buildDir)/dist.tar.gz:$(buildDir)/make-tarball plugins agents clis $(binaries)
	./$< --name $@ --prefix $(name) $(foreach item,$(binaries) $(distContents),--item $(item))
$(buildDir)/dist-race.tar.gz:$(buildDir)/make-tarball plugins makefile $(raceBinaries) $(agentBinaries) $(clientBinaries)
	./$< -name $@ --prefix $(name)-race $(foreach item,$(raceBinaries) $(distContents),--item $(item))
$(buildDir)/dist-test.tar.gz:$(buildDir)/make-tarball plugins makefile $(binaries) $(raceBinaries)
	./$< -name $@ --prefix $(name)-tests $(foreach item,$(distContents) $(distTestContents),--item $(item)) $(foreach item,,--item $(item))
$(buildDir)/dist-source.tar.gz:$(buildDir)/make-tarball $(srcFiles) $(testSrcFiles) makefile
	./$< --name $@ --prefix $(name) $(subst $(name),,$(foreach pkg,$(packages),--item ./$(subst -,/,$(pkg)))) --item ./scripts --item makefile --exclude "$(name)" --exclude "^.git/" --exclude "$(buildDir)/"
# end main build


# userfacing targets for basic build and development operations
build:$(binaries) cli agent
build-race:$(raceBinaries)
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
#    begin with configuration of dependencies
vendorDeps := github.com/Masterminds/glide
vendorDeps := $(addprefix $(gopath)/src/,$(vendorDeps))
vendor-deps:$(vendorDeps)
#   this allows us to store our vendored code in vendor and use
#   symlinks to support vendored code both in the legacy style and with
#   new-style vendor directories. When this codebase can drop support
#   for go1.4, we can delete most of this.
-include $(buildDir)/makefile.vendor
#   nested vendoring is used to support projects that have
nestedVendored := vendor/github.com/mongodb/grip
nestedVendored := $(foreach project,$(nestedVendored),$(project)/build/vendor)
$(buildDir)/makefile.vendor:$(buildDir)/render-gopath makefile
	@mkdir -p $(buildDir)
	@echo "vendorGopath := \$$(shell \$$(buildDir)/render-gopath $(nestedVendored))" >| $@
#   targets for the directory components and manipulating vendored files.
vendor-sync:$(vendorDeps)
	rm -rf vendor
	glide install -s
	rm vendor/github.com/fsouza/go-dockerclient/external/github.com/Sirupsen/logrus/terminal_freebsd.go
vendor-clean:
	rm -rf vendor/github.com/stretchr/testify/vendor/
change-go-version:
	rm -rf $(buildDir)/make-vendor $(buildDir)/render-gopath
	@$(MAKE) $(makeArgs) vendor > /dev/null 2>&1
vendor:$(buildDir)/vendor/src
	$(MAKE) $(makeArgs) -C vendor/github.com/mongodb/grip $@
$(buildDir)/vendor/src:$(buildDir)/make-vendor $(buildDir)/render-gopath
	@./$(buildDir)/make-vendor
#   targets to build the small programs used to support vendoring.
$(buildDir)/make-vendor:scripts/make-vendor.go
	@mkdir -p $(buildDir)
	go build -o $@ $<
$(buildDir)/render-gopath:scripts/render-gopath.go
	@mkdir -p $(buildDir)
	go build -o $@ $<
#   define dependencies for scripts
scripts/make-vendor.go:scripts/vendoring/vendoring.go
scripts/render-gopath.go:scripts/vendoring/vendoring.go
#   add phony targets
phony += vendor vendor-deps vendor-clean vendor-sync change-go-version
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
#  targets to compile
$(buildDir)/test.%:$(testSrcFiles)
	$(vendorGopath) go test $(if $(DISABLE_COVERAGE),,-covermode=count) -c -o $@ ./$(subst -,/,$*)
$(buildDir)/race.%:$(testSrcFiles)
	$(vendorGopath) go test -race -c -o $@ ./$(subst -,/,$*)
#  targets to run any tests in the top-level package
$(buildDir)/test.$(name):$(testSrcFiles)
	$(vendorGopath) go test $(if $(DISABLE_COVERAGE),,-covermode=count) -c -o $@ ./
$(buildDir)/race.$(name):$(testSrcFiles)
	$(vendorGopath) go test -race -c -o $@ ./
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
	$(vendorGopath) go tool cover -html=$< -o $@
# end test and coverage artifacts


# clean and other utility targets
clean:
	rm -rf $(lintDeps) $(buildDir)/test.* $(buildDir)/coverage.* $(buildDir)/race.* $(projectCleanFiles)
phony += clean
# end dependency targets

# configure phony targets
.FORCE:
.PHONY:$(phony) .FORCE
