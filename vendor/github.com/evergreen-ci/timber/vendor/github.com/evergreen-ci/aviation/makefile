name := aviation
buildDir := build
srcFiles := $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -name "*_test.go" -not -path "*\#*")
testFiles := $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -path "*\#*")

packages := $(name) services

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

# start lint setup targets
lintDeps := $(buildDir)/golangci-lint $(buildDir)/.lintSetup $(buildDir)/run-linter
$(buildDir)/golangci-lint:
	@curl --retry 10 --retry-max-time 60 -sSfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b $(buildDir) v1.30.0 >/dev/null 2>&1 && touch $@
$(buildDir)/run-linter:cmd/run-linter/run-linter.go $(buildDir)/golangci-lint
	$(gobin) build -o $@ $<
# end lint setup targets


# userfacing targets for basic build and development operations
test-%: $(buildDir)/output.%.test
	
coverage-%: $(buildDir)/output.%.coverage
	
html-coverage-%: $(buildDir)/output.%.coverage.html
	
lint-%: $(buildDir)/output.%.lint
	

compile $(buildDir): $(srcFiles)
	$(gobin) build $(subst $(name),,$(subst -,/,$(foreach target,$(packages),./$(target))))
test:$(foreach target,$(packages),$(buildDir)/output.$(target).test)
	
lint:$(foreach target,$(packages),$(buildDir)/output.$(target).lint)

.PRECIOUS:$(foreach target,$(packages),$(buildDir)/output.$(target).test)
.PRECIOUS:$(foreach target,$(packages),$(buildDir)/output.$(target).lint)

testArgs := -v
ifneq (,$(RUN_TEST))
testArgs += -run='$(RUN_TEST)'
endif
ifneq (,$(RUN_COUNT))
testArgs += -count='$(RUN_COUNT)'
endif
ifneq (,$(SKIP_LONG))
testArgs += -short
endif
ifneq (,$(DISABLE_COVERAGE))
testArgs += -cover
endif
ifneq (,$(RACE_DETECTOR))
testArgs += -race
endif

# test execution and output handlers
$(buildDir)/output.%.test:$(buildDir) .FORCE
	$(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) | tee $@
	@! grep -s -q -e "^FAIL" $@ && ! grep -s -q "^WARNING: DATA RACE" $@
$(buildDir)/output.%.coverage:$(buildDir) .FORCE
	$(gobin) test $(testArgs) -covermode=count -coverprofile ./$(if $(subst $(name),,$*),$(subst -,/,$*),) | tee $@
	@-[ -f $@ ] && $(gobin) tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/output.%.coverage.html:$(buildDir)/output.%.coverage
	$(gobin) tool cover -html=$< -o $@
#  targets to generate gotest output from the linter.
# We have to handle the PATH specially for CI, because if the PATH has a different version of Go in it, it'll break.
$(buildDir)/output.%.lint:$(buildDir)/run-linter $(buildDir)/ .FORCE
	@$(if $(GO_BIN_PATH), PATH="$(shell dirname $(GO_BIN_PATH)):$(PATH)") ./$< --output=$@ --lintBin=$(buildDir)/golangci-lint --packages='$*'
#  targets to process and generate coverage reports
# end test and coverage artifacts


# start vendoring configuration
#    begin with configuration of dependencies
vendor-clean:
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/pkg/errors/
	find vendor/ -name "*.gif" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" -o -name "*testdata*" | xargs rm -rf
phony += vendor-clean
# end vendoring tooling configuration


clean:
	rm -rf $(lintDeps)
clean-results:
	rm -rf $(buildDir)/output.*

phony += lint build test coverage coverage-html
# end front-ends


# configure phony targets
.FORCE:
.PHONY:$(phony) .FORCE
