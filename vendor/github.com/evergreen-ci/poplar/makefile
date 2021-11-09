buildDir := build
name := poplar
packages := poplar rpc rpc-internal

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


# start lint setup targets
lintDeps := $(buildDir)/run-linter $(buildDir)/golangci-lint
$(buildDir)/golangci-lint:
	@curl --retry 10 --retry-max-time 60 -sSfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b $(buildDir) v1.40.0 >/dev/null 2>&1
$(buildDir)/run-linter:cmd/run-linter/run-linter.go $(buildDir)/golangci-lint
	@$(gobin) build -o $@ $<
# end lint setup targets

coverageOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage)
coverageHtmlOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage.html)
lintOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).lint)
testOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).test)

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

benchPattern := ./

test:$(testOutput)
	
.PHONY: benchmark
benchmark:
	$(gobin) test $(testArgs) -bench=$(benchPattern) $(if $(RUN_TEST),, -run=^^$$) | tee $(buildDir)/bench.out
coverage:$(coverageOutput)
	
coverage-html:$(coverageHtmlOutput)
	
lint:$(lintOutput)
	

phony += lint $(buildDir) test coverage coverage-html
.PRECIOUS:$(coverageOutput) $(coverageHtmlOutput) $(lintOutput) $(testOutput)


compile $(buildDir):
	$(gobin) build $(subst $(name),,$(subst -,/,$(foreach target,$(packages),./$(target))))
$(buildDir)/output.%.test: .FORCE
	$(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) | tee $@
	@!( grep -s -q "^FAIL" $@ && grep -s -q "^WARNING: DATA RACE" $@)
	@(grep -s -q "^PASS" $@ || grep -s -q "no test files" $@)
#  targets to process and generate coverage reports
$(buildDir)/output.%.coverage: .FORCE
	$(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) -covermode=count -coverprofile $@ | tee $(buildDir)/output.$*.test
	@-[ -f $@ ] && $(gobin) tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/output.%.coverage.html: $(buildDir)/output.%.coverage .FORCE
	$(gobin) tool cover -html=$< -o $@
#  targets to generate gotest output from the linter.
# We have to handle the PATH specially for CI, because if the PATH has a different version of Go in it, it'll break.
$(buildDir)/output.%.lint: $(buildDir)/run-linter .FORCE
	@$(if $(GO_BIN_PATH), PATH="$(shell dirname $(GO_BIN_PATH)):$(PATH)") ./$< --output=$@ --lintBin=$(buildDir)/golangci-lint --packages='$*'
# end test and coverage artifacts

.FORCE:

.PHONY: $(phony) .FORCE

proto:vendor/cedar.proto
	@mkdir -p rpc/internal
	protoc --go_out=plugins=grpc:rpc/internal *.proto
	protoc --go_out=plugins=grpc:rpc/internal vendor/cedar.proto
	protoc --go_out=plugins=grpc:collector *.proto
	mv rpc/internal/vendor/cedar.pb.go rpc/internal/cedar.pb.go
clean:
	rm -rf $(lintDeps) rpc/internal/*.pb.go
clean-results:
	rm -rf $(buildDir)/output.*

vendor/cedar.proto:
	curl -L https://raw.githubusercontent.com/evergreen-ci/cedar/master/perf.proto -o $@
vendor:
	glide install -s


.PHONY:vendor
vendor-clean:
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/google/uuid
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/montanaflynn/stats/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/mongodb/grip/vendor/golang.org/x/sys/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/stretchr/testify
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/papertrail/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/go.mongodb.org/mongo-driver/
	rm -rf vendor/github.com/mongodb/grip/buildscripts/
	rm -rf vendor/github.com/evergreen-ci/pail/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/pail/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/pail/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/evergreen-ci/pail/vendor/go.mongodb.org/mongo-driver/
	rm -rf vendor/github.com/papertrail/go-tail/vendor/golang.org/x/sys/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/google/uuid/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/mongodb/amboy/vendor/gopkg.in/yaml.v2/
	rm -rf vendor/github.com/mongodb/amboy/vendor/go.mongodb.org/mongo-driver/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/google.golang.org/grpc/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/github.com/stretchr/testify/
	find vendor/ -name "*.gif" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" -o -name "*.dat" -o -name "*testdata" | xargs rm -rf

# convenience targets for runing tests and coverage tasks on a
# specific package.
test-%:$(buildDir)/output.%.test
	@grep -s -q -e "^PASS" $<
coverage-%:$(buildDir)/output.%.coverage
	@grep -s -q -e "^PASS" $(buildDir)/output.$*.test
html-coverage-%:$(buildDir)/output.%.coverage.html
	@grep -s -q -e "^PASS" $(buildDir)/output.$*.test
lint-%:$(buildDir)/output.%.lint
	@grep -v -s -q "^--- FAIL" $<
# end convienence targets

