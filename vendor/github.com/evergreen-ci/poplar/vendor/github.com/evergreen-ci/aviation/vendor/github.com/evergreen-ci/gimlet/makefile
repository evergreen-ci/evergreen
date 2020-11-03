# start project configuration
name := gimlet
buildDir := build
packages := $(name) ldap
orgPath := github.com/evergreen-ci
projectPath := $(orgPath)/$(name)
# end project configuration

# go environment configuration
ifneq (,$(GO_BIN_PATH))
gobin := $(GO_BIN_PATH)
else
gobin := go
endif

goos := $(shell $(gobin) env GOOS)
goarch := $(shell $(gobin) env GOARCH)
gopath := $(GOPATH)
ifeq ($(OS),Windows_NT)
gopath := $(shell cygpath -m $(gopath))
endif
# end go environment configuration


# start linting configuration
#   package, testing, and linter dependencies specified
#   separately. This is a temporary solution: eventually we should
#   vendorize all of these dependencies.
lintDeps := github.com/alecthomas/gometalinter
lintDeps += github.com/richardsamuels/evg-lint/...
#   include test files and give linters 40s to run to avoid timeouts
lintArgs := --tests --deadline=1m --vendor --aggregate --sort=line
#   gotype produces false positives because it reads .a files which
#   are rarely up to date.
lintArgs += --disable="gotype" --enable=goimports
lintArgs += --vendored-linters --enable-gc
#  add and configure additional linters
lintArgs += --line-length=100 --dupl-threshold=175 --cyclo-over=17
#  two similar functions triggered the duplicate warning, but they're not.
lintArgs += --exclude="duplicate of registry.go"
lintArgs += --exclude="don.t use underscores.*_DependencyState.*"
#  golint doesn't handle splitting package comments between multiple files.
lintArgs += --exclude="package comment should be of the form \"Package .* \(golint\)"
lintArgs += --exclude="error return value not checked \(defer .* \(errcheck\)$$"
lintArgs += --exclude="declaration of \"assert\" shadows declaration at .*_test.go:"
lintArgs += --exclude="declaration of \"require\" shadows declaration at .*_test.go:"
lintArgs += --linter="evg:$(gopath)/bin/evg-lint:PATH:LINE:COL:MESSAGE" --enable=evg
# end lint suppressions

######################################################################
##
## Everything below this point is generic, and does not contain
## project specific configuration. (with one noted case in the "build"
## target for library-only projects)
##
######################################################################


# start dependency installation tools
#   implementation details for being able to lazily install dependencies
gopath := $(shell go env GOPATH)
lintDeps := $(addprefix $(gopath)/src/,$(lintDeps))
srcFiles := makefile $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -name "*_test.go")
testSrcFiles := makefile $(shell find . -name "*.go" -not -path "./$(buildDir)/*")
testOutput := $(subst -,/,$(foreach target,$(packages),$(buildDir)/test.$(target).out))
raceOutput := $(subst -,/,$(foreach target,$(packages),$(buildDir)/race.$(target).out))
coverageOutput := $(subst -,/,$(foreach target,$(packages),$(buildDir)/coverage.$(target).out))
coverageHtmlOutput := $(subst -,/,$(foreach target,$(packages),$(buildDir)/coverage.$(target).html))
$(gopath)/src/%:
	@-[ ! -d $(gopath) ] && mkdir -p $(gopath) || true
	$(gobin) get $(subst $(gopath)/src/,,$@)
# end dependency installation tools


# lint setup targets
lintDeps := $(addprefix $(gopath)/src/,$(lintDeps))
lintTargets := $(foreach target,$(packages),lint-$(target))
$(buildDir)/.lintSetup:$(lintDeps)
	@mkdir -p $(buildDir)
	@$(gopath)/bin/gometalinter --force --install >/dev/null && touch $@
$(buildDir)/run-linter:buildscripts/run-linter.go
	$(gobin) build -o $@ $<
.PRECIOUS:$(buildDir)/output.lint
# end lint setup targets

# userfacing targets for basic build and development operations
lint:$(buildDir)/output.lint
lint-deps:$(lintDeps)
build:$(deps) $(srcFiles) $(gopath)/src/$(projectPath)
	@mkdir -p $(buildDir)
	$(gobin) build ./.
build-race:$(deps) $(srcFiles) $(gopath)/src/$(projectPath)
	$(gobin) build -race $(subst -,/,$(foreach pkg,$(packages),./$(pkg)))
test:$(testOutput)
race:$(raceOutput)
coverage:$(coverageOutput)
coverage-html:$(coverageHtmlOutput)
phony := build build-race race test coverage coverage-html
phony += deps test-deps lint-deps
.PRECIOUS: $(testOutput) $(raceOutput) $(coverageOutput) $(coverageHtmlOutput)
# end front-ends


# implementation details for building the binary and creating a
# convienent link in the working directory
$(gopath)/src/$(orgPath):
	@mkdir -p $@
$(gopath)/src/$(projectPath):$(gopath)/src/$(orgPath)
	@[ -L $@ ] || ln -s $(shell pwd) $@
$(buildDir)/$(name):$(gopath)/src/$(projectPath) $(srcFiles) $(deps)
	$(gobin) build -o $@ main/$(name).go
$(buildDir)/$(name).race:$(gopath)/src/$(projectPath) $(srcFiles) $(deps)
	$(gobin) build -race -o $@ main/$(name).go
# end main build


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


# start test and coverage artifacts
#    tests have compile and runtime deps. This varable has everything
#    that the tests actually need to run. (The "build" target is
#    intentional and makes these targets rerun as expected.)
testRunDeps := $(testSrcFiles) build
testArgs := -test.v
ifneq (,$(RUN_TEST))
testArgs += -test.run='$(RUN_TEST)'
endif
ifneq (,$(RUN_CASE))
testArgs += -testify.m='$(RUN_CASE)'
endif
ifneq (,$(RUN_COUNT))
testArgs += -test.count='$(RUN_COUNT)'
endif
ifneq (,$(TEST_TIMEOUT))
testArgs += -test.timeout=$(TEST_TIMEOUT)
else
testArgs += -test.timeout=10m
endif
#    implementation for package coverage and test running,mongodb to produce
#    and save test output.
$(buildDir)/coverage.%.html:$(buildDir)/coverage.%.out
	$(gobin) tool cover -html=$(buildDir)/coverage.$(subst /,-,$*).out -o $(buildDir)/coverage.$(subst /,-,$*).html
$(buildDir)/coverage.%.out:$(testRunDeps)
	GOPATH=$(gopath) $(gobin) test $(testArgs) -covermode=count -coverprofile=$(buildDir)/coverage.$(subst /,-,$*).out $(projectPath)/$(subst -,/,$*)
	@-[ -f $(buildDir)/coverage.$(subst /,-,$*).out ] && go tool cover -func=$(buildDir)/coverage.$(subst /,-,$*).out | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/coverage.$(name).out:$(testRunDeps)
	GOPATH=$(gopath) $(gobin) test -covermode=count -coverprofile=$@ $(projectPath)
	@-[ -f $@ ] && go tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/test.%.out:$(testRunDeps) .FORCE
	GOPATH=$(gopath) $(gobin) test $(testArgs) ./$(subst -,/,$*) | tee $(buildDir)/test.$(subst /,-,$*).out
$(buildDir)/race.%.out:$(testRunDeps) .FORCE
	GOPATH=$(gopath) $(gobin) test $(testArgs) -race ./$(subst -,/,$*) | tee $(buildDir)/race.$(subst /,-,$*).out
$(buildDir)/test.$(name).out:$(testRunDeps) .FORCE
	GOPATH=$(gopath) $(gobin) test $(testArgs) ./ | tee $@
$(buildDir)/race.$(name).out:$(testRunDeps) .FORCE
	GOPATH=$(gopath) $(gobin) test $(testArgs) -race ./ | tee $@
#  targets to generate gotest output from the linter.
$(buildDir)/output.%.lint:$(buildDir)/run-linter $(testSrcFiles)  $(buildDir)/.lintSetup .FORCE
	@./$< --output=$@ --lintArgs='$(lintArgs)' --packages='$*'
$(buildDir)/output.lint:$(buildDir)/run-linter $(buildDir)/.lintSetup .FORCE
	@./$< --output="$@" --lintArgs='$(lintArgs)' --packages="$(packages)"
# end test and coverage artifacts


# start vendoring configuration
vendor-clean:
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/pkg/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/stretchr/
	rm -rf vendor/gopkg.in/asn1-ber.v1/tests/
	find vendor/ -name "*.gif" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" -o -name "*.dat" -o -name "*testdata" | xargs rm -rf
phony += vendor-clean
# end vendoring tooling configuration


# clean and other utility targets
clean:
	rm -rf $(name) $(lintDeps) $(buildDir)/test.* $(buildDir)/coverage.* $(buildDir)/race.*
phony += clean
# end dependency targets

# configure phony targets
.FORCE:
.PHONY:$(phony)
