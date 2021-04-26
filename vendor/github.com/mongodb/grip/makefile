# project configuration
name := grip
buildDir := build
packages := recovery logging message send slogger $(name)
orgPath := github.com/mongodb
projectPath := $(orgPath)/$(name)
# end project configuration

# start environment setup
gobin := $(GO_BIN_PATH)
ifeq (,$(gobin))
	gobin := go
endif
gocache := $(abspath $(buildDir)/.cache)
gopath := $(GOPATH)
goroot := $(GOROOT)
ifeq ($(OS),Windows_NT)
	gocache := $(shell cygpath -m $(gocache))
	gopath := $(shell cygpath -m $(gopath))
	goroot := $(shell cygpath -m $(goroot))
endif
export GOCACHE := $(gocache)
export GOPATH := $(gopath)
export GOROOT := $(goroot)
# end environment setup

# Ensure the build directory exists, since most targets require it.
$(shell mkdir -p $(buildDir))

# lint setup targets
lintDeps := $(buildDir)/golangci-lint $(buildDir)/run-linter
$(buildDir)/golangci-lint:
	@curl --retry 10 --retry-max-time 60 -sSfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b $(buildDir) v1.30.0 >/dev/null 2>&1
$(buildDir)/run-linter:cmd/run-linter/run-linter.go $(buildDir)/golangci-lint
	$(gobin) build -o $@ $<
# end lint setup targets
#
######################################################################
##
## Everything below this point is generic, and does not contain
## project specific configuration. (with one noted case in the "build"
## target for library-only projects)
##
######################################################################


# start dependency installation tools
#   implementation details for being able to lazily install dependencies
testOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).test)
raceOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).race)
coverageOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage)
coverageHtmlOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage.html)
# end dependency installation tools


# userfacing targets for basic build and development operations
lint:$(foreach target,$(packages),lint-$(target))
build: $(gopath)/src/$(projectPath)
	$(gobin) build $(subst $(name),,$(subst -,/,$(foreach pkg,$(packages),./$(pkg))))
test:$(testOutput)
race:$(raceOutput)
coverage:$(coverageOutput)
coverage-html:$(coverageHtmlOutput)
phony := lint build build-race race test benchmark-send coverage coverage-html
.PRECIOUS:$(testOutput) $(raceOutput) $(coverageOutput) $(coverageHtmlOutput)
.PRECIOUS:$(foreach target,$(packages),$(buildDir)/output.$(target).test)
.PRECIOUS:$(foreach target,$(packages),$(buildDir)/output.$(target).lint)
# end front-ends


# implementation details for building the binary and creating a
# convienent link in the working directory
# convenience targets for running tests and coverage tasks on a
# specific package.
makeArgs := --no-print-directory
test-%:$(buildDir)/output.%.test
	@grep -s -q -e "^PASS" $<
benchmark-send:
	$(gobin) test -v -bench=$(if $(RUN_BENCH),$(RUN_BENCH),BenchmarkAllSenders) ./send/ ./send/benchmark/ -run=^^$$
coverage-%:$(buildDir)/output.%.coverage
	@grep -s -q -e "^PASS" $(buildDir)/output.$*.test
html-coverage-%:$(buildDir)/output.%.coverage $(buildDir)/output.%.coverage.html
	@grep -s -q -e "^PASS" $(buildDir)/output.$*.test
lint-%:$(buildDir)/output.%.lint
	@grep -v -s -q "^--- FAIL" $<
# end convienence targets


# start test and coverage artifacts
#    tests have compile and runtime deps. This varable has everything
#    that the tests actually need to run. (The "build" target is
#    intentional and makes these targets rerun as expected.)
testArgs := -test.v --test.timeout=5m
ifneq (,$(RUN_TEST))
testArgs += -test.run='$(RUN_TEST)'
endif
ifneq (,$(RUN_CASE))
testArgs += -testify.m='$(RUN_CASE)'
endif
$(buildDir)/output.%.test: .FORCE
	$(gobin) test $(if $(DISABLE_COVERAGE),,-covermode=count) $(testArgs) ./$(subst -,/,$*) | tee $@
$(buildDir)/output.$(name).test: .FORCE
	$(gobin) test $(testArgs) ./ | tee $@

$(buildDir)/output.%.race: .FORCE
	$(gobin) test -race $(testArgs) ./$(subst -,/,$*) 2>&1 | tee $@
$(buildDir)/output.$(name).race: .FORCE
	$(gobin) test -race $(testArgs) ./ 2>&1 | tee $@
#  targets to generate gotest output from the linter.
# We have to handle the PATH specially for CI, because if the PATH has a different version of Go in it, it'll break.
$(buildDir)/output.%.lint:$(buildDir)/run-linter .FORCE
	@$(if $(GO_BIN_PATH), PATH="$(shell dirname $(GO_BIN_PATH)):$(PATH)") ./$< --output=$@ --lintBin="$(buildDir)/golangci-lint" --packages='$*'
#  targets to process and generate coverage reports
$(buildDir)/output.%.coverage: .FORCE
	$(gobin) test $(testArgs) -test.coverprofile=$@ | tee $(subst coverage,test,$@)
	@-[ -f $@ ] && $(gobin) tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/output.%.coverage.html:$(buildDir)/output.%.coverage
	$(gobin) tool cover -html=$< -o $@
# end test and coverage artifacts


# start vendoring configuration
#    begin with configuration of dependencies
vendor-clean:
	rm -rf vendor/github.com/mattn/go-xmpp/_example/
	rm -rf vendor/github.com/bluele/slack/examples/
	find vendor/ -name "*.go" | xargs gofmt -w -r '"golang.org/x/net/context" -> "context"'
	find vendor/github.com/shirou/gopsutil/ -name "*.go" | xargs -n 1 gofmt -w || true
	find vendor/ -name "*.gif" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" -o -name "*.dat" -o -name "*testdata" | xargs rm -fr
#   add phony targets
phony += vendor-clean
# end vendoring tooling configuration


# clean and other utility targets
clean:
	rm -rf $(name) $(lintDeps)
clean-results:
	$(buildDir)/output.*
phony += clean
# end dependency targets


# configure phony targets
.FORCE:
.PHONY:$(phony) .FORCE
