# start project configuration
name := timber
buildDir := build
packages := $(name) buildlogger systemmetrics testresults testutil perf
testPackages := timber buildlogger systemmetrics testresults perf
compilePackages := $(subst $(name),,$(subst -,/,$(foreach target,$(packages),./$(target))))
projectPath := github.com/evergreen-ci/timber
# end project configuration

# start environment setup
gobin := go
ifneq (,$(GOROOT))
gobin := $(GOROOT)/bin/go
endif

gocache := $(GOCACHE)
ifeq (,$(gocache))
gocache := $(abspath $(buildDir)/.cache)
endif
lintCache := $(GOLANGCI_LINT_CACHE)
ifeq (,$(lintCache))
lintCache := $(abspath $(buildDir)/.lint-cache)
endif

ifeq ($(OS),Windows_NT)
gobin := $(shell cygpath $(gobin))
gocache := $(shell cygpath -m $(gocache))
lintCache := $(shell cygpath -m $(lintCache))
export GOPATH := $(shell cygpath -m $(GOPATH))
export GOROOT := $(shell cygpath -m $(GOROOT))
endif

ifneq ($(gocache),$(GOCACHE))
export GOCACHE := $(gocache)
endif
ifneq ($(lintCache),$(GOLANGCI_LINT_CACHE))
export GOLANGCI_LINT_CACHE := $(lintCache)
endif

export GO111MODULE := off
ifneq (,$(RACE_DETECTOR))
# cgo is required for using the race detector.
export CGO_ENABLED=1
else
export CGO_ENABLED=0
endif
# end environment setup

# Ensure the build directory exists, since most targets require it.
$(shell mkdir -p $(buildDir))

.DEFAULT_GOAL := compile

# start lint setup targets
$(buildDir)/golangci-lint:
	@curl --retry 10 --retry-max-time 60 -sSfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b $(buildDir) v1.40.0 >/dev/null 2>&1
$(buildDir)/run-linter:  cmd/run-linter/run-linter.go $(buildDir)/golangci-lint
	@$(gobin) build -o $@ $<
# end lint setup targets

# start output files
testOutput := $(foreach target,$(testPackages),$(buildDir)/output.$(target).test)
lintOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).lint)
coverageOutput := $(foreach target,$(testPackages),$(buildDir)/output.$(target).coverage)
coverageHtmlOutput := $(foreach target,$(testPackages),$(buildDir)/output.$(target).coverage.html)
.PRECIOUS:$(coverageOutput) $(coverageHtmlOutput) $(lintOutput) $(testOutput)
# end output files

# start basic development operations
compile:
	$(gobin) build $(compilePackages)
test: $(testOutput)
	
coverage: $(coverageOutput)
	
coverage-html: $(coverageHtmlOutput)
	
lint: $(lintOutput)
	
phony += compile lint test coverage coverage-html

# start convenience targets for running tests and coverage tasks on a
# specific package.
test-%:$(buildDir)/output.%.test
	
coverage-%:$(buildDir)/output.%.coverage
	
html-coverage-%:$(buildDir)/output.%.coverage.html
	
lint-%:$(buildDir)/output.%.lint
	
# end convenience targets
# end basic development operations

# start test and coverage artifacts
testArgs := -v
ifeq (,$(DISABLE_COVERAGE))
	testArgs += -cover
endif
ifneq (,$(RACE_DETECTOR))
	testArgs += -race
endif
ifneq (,$(RUN_COUNT))
	testArgs += -count=$(RUN_COUNT)
endif
ifneq (,$(RUN_TEST))
	testArgs += -run='$(RUN_TEST)'
endif
ifneq (,$(SKIP_LONG))
	testArgs += -short
endif
$(buildDir)/output.%.test: .FORCE
	$(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) | tee $@
	@grep -s -q -e "^PASS" $@
$(buildDir)/output.%.coverage: .FORCE
	$(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) -covermode=count -coverprofile $@ | tee $(buildDir)/output.$*.test
	@-[ -f $@ ] && $(gobin) tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
	@grep -s -q -e "^PASS" $(subst coverage,test,$@)
$(buildDir)/output.%.coverage.html: $(buildDir)/output.%.coverage
	$(gobin) tool cover -html=$< -o $@

ifneq (go,$(gobin))
# We have to handle the PATH specially for linting in CI, because if the PATH has a different version of the Go
# binary in it, the linter won't work properly.
lintEnvVars := PATH="$(shell dirname $(gobin)):$(PATH)"
endif
$(buildDir)/output.%.lint: $(buildDir)/run-linter .FORCE
	@$(lintEnvVars) ./$< --output=$@ --lintBin=$(buildDir)/golangci-lint --packages='$*'
# end test and coverage artifacts

# start vendoring configuration
vendor:
	glide install -s
vendor-clean:
	rm -rf vendor/github.com/mongodb/grip/vendor/golang.org/x/oauth2/
	rm -rf vendor/github.com/mongodb/grip/vendor/golang.org/x/sys/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/github.com/evergreen-ci/gimlet/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/github.com/jpillora/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/google.golang.org/grpc/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/google.golang.org/genproto/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/evergreen-ci/gimlet/vendor/go.mongodb.org/mongo-driver/
	rm -rf vendor/github.com/evergreen-ci/utility/gitignore.go
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/stretchr/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/pkg/errors/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/golang.org/x/net/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/golang.org/x/sys/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/golang.org/x/text/
	rm -rf vendor/go.mongodb.org/mongo-driver/data/
	find vendor/ -name "*.gif" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" -o -name "*.dat" -o -name "*testdata" | xargs rm -rf
	find vendor/ -name .git | xargs rm -rf
phony += vendor-clean
# end vendoring configuration

# start cleanup targets
clean:
	rm -rf $(buildDir)
clean-results:
	rm -rf $(buildDir)/output.*
# end cleanup targets

# configure phony targets
.FORCE:
.PHONY: $(phony)
