# start project configuration
name := amboy
buildDir := build
packages := $(name) dependency job registry pool queue rest logger management cli
orgPath := github.com/mongodb
projectPath := $(orgPath)/$(name)
# end project configuration


 # start environment setup
gobin := ${GO_BIN_PATH}
ifeq (,$(gobin))
gobin := go
endif
gopath := ${GOPATH}
gocache := $(abspath $(buildDir)/.cache)
ifeq ($(OS),Windows_NT)
gocache := $(shell cygpath -m $(gocache))
gopath := $(shell cygpath -m $(gopath))
endif
goEnv := GOPATH=$(gopath) GOCACHE=$(gocache) $(if ${GO_BIN_PATH},PATH="$(shell dirname ${GO_BIN_PATH}):${PATH}")
# end environment setup


# start lint setup targets
lintDeps := $(buildDir)/.lintSetup $(buildDir)/run-linter $(buildDir)/golangci-lint
$(buildDir)/.lintSetup:$(buildDir)/golangci-lint
	@touch $@
$(buildDir)/golangci-lint:
	@mkdir -p $(buildDir)
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/76a82c6ed19784036bbf2d4c84d0228ca12381a4/install.sh | sh -s -- -b $(buildDir) v1.23.8 >/dev/null 2>&1
$(buildDir)/run-linter:buildscripts/run-linter.go $(buildDir)/.lintSetup $(buildDir)
	@$(goEnv) $(gobin) build -o $@ $<
# end lint setup targets


# benchmark setup targets
benchmarks:$(buildDir)/run-benchmarks $(buildDir) .FORCE
	$(goEnv) ./$(buildDir)/run-benchmarks $(run-benchmark)
$(buildDir)/run-benchmarks:cmd/run-benchmarks/run-benchmarks.go $(buildDir)
	$(goEnv) $(gobin) build -o $@ $<
# end benchmark setup targets


######################################################################
##
## Everything below this point is generic, and does not contain
## project specific configuration. (with one noted case in the "build"
## target for library-only projects)
##
######################################################################

_compilePackages := $(subst $(name),,$(subst -,/,$(foreach target,$(packages),./$(target))))
coverageOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage)
coverageHtmlOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage.html)

# start dependency installation tools
#   implementation details for being able to lazily install dependencies
testOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).test)
coverageOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage)
coverageHtmlOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage.html)
# end dependency installation tools


# userfacing targets for basic build and development operations
lint:$(foreach target,$(packages),$(buildDir)/output.$(target).lint)
test:$(foreach target,$(packages),$(buildDir)/output.$(target).test)
coverage:$(buildDir) $(coverageOutput)
coverage-html:$(buildDir) $(coverageHtmlOutput)
compile $(buildDir):
	@mkdir -p $(buildDir)
	$(goEnv) $(gobin) build $(_compilePackages)
compile-base:
	$(goEnv) $(gobin) build  ./
# convenience targets for runing tests and coverage tasks on a
# specific package.
test-%:$(buildDir)/output.%.test
	
coverage-%:$(buildDir)/output.%.coverage
	
html-coverage-%:$(buildDir)/output.%.coverage.html
	
lint-%:$(buildDir)/output.%.lint
	
# end convienence targets
phony := lint build build-race race test coverage coverage-html
.PRECIOUS:$(testOutput) $(coverageOutput) $(coverageHtmlOutput)
.PRECIOUS:$(foreach target,$(packages),$(buildDir)/test.$(target))
.PRECIOUS:$(foreach target,$(packages),$(buildDir)/output.$(target).lint)
.PRECIOUS:$(buildDir)/output.lint
# end front-end


# implementation details for building the binary and creating a
# convienent link in the working directory
$(gopath)/src/$(orgPath):
	@mkdir -p $@
$(gopath)/src/$(projectPath):$(gopath)/src/$(orgPath)
	@[ -L $@ ] || ln -s $(shell pwd) $@
# end main build



# start test and coverage artifacts
#    tests have compile and runtime deps. This varable has everything
#    that the tests actually need to run. (The "build" target is
#    intentional and makes these targets rerun as expected.)
testArgs := -test.v --test.timeout=1h
ifneq (,$(RUN_TEST))
testArgs += -test.run='$(RUN_TEST)'
endif
ifneq (,$(RUN_CASE))
testArgs += -testify.m='$(RUN_CASE)'
endif
ifneq (,$(RUN_COUNT))
testArgs += -test.count='$(RUN_COUNT)'
endif
ifneq (,$(RACE_DETECTOR))
testArgs += -race
endif
ifneq (,$(DISABLE_COVERAGE))
testArgs += -cover
endif
ifneq (,$(SKIP_LONG))
testArgs += -short
endif
#    implementation for package coverage and test running,mongodb to produce
#    and save test output.
$(buildDir)/:
	@mkdir -p $@
$(buildDir)/output.%.test:.FORCE
	@mkdir -p $(buildDir)/
	$(goEnv) $(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) | tee $@
	@!( grep -s -q "^FAIL" $@ && grep -s -q "^WARNING: DATA RACE" $@)
	@(grep -s -q "^PASS" $@ || grep -s -q "no test files" $@)
$(buildDir)/output.%.coverage:.FORCE
	@mkdir -p $(buildDir)/
	$(goEnv) $(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) -coverprofile $@ | tee $(buildDir)/output.$*.test
	@-[ -f $@ ] && $(gobin) tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/output.%.coverage.html:$(buildDir)/output.%.coverage
	$(goEnv) $(gobin) tool cover -html=$< -o $@
#  targets to generate gotest output from the linter.
$(buildDir)/output.%.lint:$(buildDir)/run-linter .FORCE
	@mkdir -p $(buildDir)/
	@$(goEnv) ./$< --output=$@ --lintBin=$(buildDir)/golangci-lint --packages='$*'
# end test and coverage artifacts


# start vendoring configuration
vendor-clean:
	rm -rf vendor/gopkg.in/mgo.v2/harness/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/google/uuid
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/evergreen-ci/pail/vendor/github.com/aws/aws-sdk-go/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/mongodb/amboy/
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/pkg/errors/
	rm -rf vendor/gopkg.in/mgo.v2/harness/
	rm -rf vendor/gopkg.in/mgo.v2/testdb/
	rm -rf vendor/gopkg.in/mgo.v2/testserver/
	rm -rf vendor/gopkg.in/mgo.v2/internal/json/testdata
	rm -rf vendor/gopkg.in/mgo.v2/.git/
	rm -rf vendor/gopkg.in/mgo.v2/txn/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/gopkg.in/yaml.v2
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/davecgh/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/stretchr/testify/
	rm -rf vendor/go.mongodb.org/mongo-driver/data/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/pmezard/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/google/go-cmp/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/kr/
	find vendor/ -name "*.gif" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" -o -name "*.dat" -o -name "*testdata" | xargs rm -rf
#   define dependencies for buildscripts
phony += vendor-clean
# end vendoring tooling configuration


# clean and other utility targets
clean:
	rm -rf $(name) $(lintDeps) $(buildDir)/output.*
phony += clean
# end dependency targets

# configure phony targets
.FORCE:
.PHONY:$(phony)
