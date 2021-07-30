name := cocoa
projectPath := github.com/evergreen-ci/cocoa
buildDir := build
testPackages := $(name) ecs secret mock awsutil
allPackages := $(testPackages) internal-testcase internal-testutil
lintPackages := $(allPackages)

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
export GO111MODULE := off
# end environment setup

# Ensure the build directory exists, since most targets require it.
$(shell mkdir -p $(buildDir))

# start testing convenience targets
testOutput := $(foreach target,$(testPackages),$(buildDir)/output.$(target).test)
lintOutput := $(foreach target,$(lintPackages),$(buildDir)/output.$(target).lint)
coverageOutput := $(foreach target,$(testPackages),$(buildDir)/output.$(target).coverage)
coverageHtmlOutput := $(foreach target,$(testPackages),$(buildDir)/output.$(target).coverage.html)
.PRECIOUS: $(coverageOutput) $(coverageHtmlOutput) $(lintOutput) $(testOutput)

.DEFAULT_GOAL := compile

compile:
	$(gobin) build $(subst $(name),,$(subst -,/,$(foreach target,$(allPackages),./$(target))))
test: $(testOutput)
lint: $(lintOutput)
coverage: $(coverageOutput)
coverage-html: $(coverageHtmlOutput)
phony += lint test coverage coverage-html

test-%: $(buildDir)/output.%.test
	
coverage-%: $(buildDir)/output.%.coverage
	
coverage-html-%: $(buildDir)/output.%.coverage.html
	
lint-%: $(buildDir)/output.%.lint
	
# end testing convenience targets


# start lint setup targets
lintDeps := $(buildDir)/run-linter $(buildDir)/golangci-lint
$(buildDir)/golangci-lint:
	@curl --retry 10 --retry-max-time 60 -sSfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b $(buildDir) v1.40.0 >/dev/null 2>&1
$(buildDir)/run-linter: cmd/run-linter/run-linter.go $(buildDir)/golangci-lint
	@$(gobin) build -o $@ $<
# end lint setup targets

# start lint, test, and coverage artifacts
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

$(buildDir)/output.%.test: .FORCE
	$(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) | tee $@
	@!(grep -s -q "^FAIL" $@ || grep -s -q "^WARNING: DATA RACE" $@ || grep -s -q "no test files" $@)
$(buildDir)/output.%.coverage: .FORCE
	$(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) -covermode=count -coverprofile $@ | tee $(buildDir)/output.$*.test
	@[ -f $@ ] && $(gobin) tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/output.%.coverage.html: $(buildDir)/output.%.coverage .FORCE
	$(gobin) tool cover -html=$< -o $@
# We have to handle the PATH specially for CI, because if the PATH has a different version of Go in it, it'll break.
$(buildDir)/output.%.lint: $(buildDir)/run-linter .FORCE
	@$(if $(GO_BIN_PATH), PATH="$(shell dirname $(GO_BIN_PATH)):$(PATH)") ./$< --output=$@ --lintBin=$(buildDir)/golangci-lint --packages='$*'
# end lint, test, and coverage artifacts

vendor-clean:
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/utility/gitignore.go
	rm -rf vendor/github.com/evergreen-ci/utility/parsing.go
	rm -rf vendor/github.com/jmespath/go-jmespath/internal/testify/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/stretchr/objx/vendor/github.com/davecgh/go-spew/
	rm -rf vendor/github.com/stretchr/objx/vendor/github.com/stretchr/testify/
	find vendor/ -name "*.gif" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" -o -name "*testdata*" | xargs rm -rf
	find vendor/ -type d -empty | xargs rm -rf
	find vendor/ -type d -name '.git' | xargs rm -rf

clean:
	rm -rf $(buildDir)

clean-results:
	rm -rf $(buildDir)/output.*

.FORCE:

.PHONY: $(phony) .FORCE
