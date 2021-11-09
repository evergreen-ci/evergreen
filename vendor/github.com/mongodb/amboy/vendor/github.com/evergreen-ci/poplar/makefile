buildDir := build

packages := ./ ./rpc ./rpc/internal
lintPackages := poplar rpc rpc-internal
# override the go binary path if set
ifneq (,$(GO_BIN_PATH))
gobin := $(GO_BIN_PATH)
else
gobin := go
endif
gopath := $(GOPATH)
ifeq ($(OS),Windows_NT)
ifneq ($(gopath),)
gopath := $(shell cygpath -m $(gopath))
endif
endif
goEnv := GOPATH=$(gopath) $(if $(GO_BIN_PATH),PATH="$(shell dirname $(GO_BIN_PATH)):$(PATH)")


# start lint setup targets
lintDeps := $(buildDir)/.lintSetup $(buildDir)/run-linter $(buildDir)/golangci-lint
$(buildDir)/.lintSetup:$(buildDir)/golangci-lint
	@touch $@
$(buildDir)/golangci-lint:$(buildDir)
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/76a82c6ed19784036bbf2d4c84d0228ca12381a4/install.sh | sh -s -- -b $(buildDir) v1.23.8 >/dev/null 2>&1
$(buildDir)/run-linter:cmd/run-linter/run-linter.go $(buildDir)/.lintSetup $(buildDir)
	@$(goEnv) $(gobin) build -o $@ $<
# end lint setup targets


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

benchPattern := ./

compile:
	$(goEnv) $(gobin) build $(packages)
race:
	@mkdir -p $(buildDir)
	$(goEnv) $(gobin) test $(testArgs) -race $(packages) | tee $(buildDir)/race.out
	@grep -s -q -e "^PASS" $(buildDir)/race.out && ! grep -s -q "^WARNING: DATA RACE" $(buildDir)/race.out
test:
	@mkdir -p $(buildDir)
	$(goEnv) $(gobin) test $(testArgs) $(if $(DISABLE_COVERAGE),, -cover) $(packages) | tee $(buildDir)/test.out
	@grep -s -q -e "^PASS" $(buildDir)/test.out
.PHONY: benchmark
benchmark:
	@mkdir -p $(buildDir)/
	$(goEnv) $(gobin) test $(testArgs) -bench=$(benchPattern) $(if $(RUN_TEST),, -run=^^$$) | tee $(buildDir)/bench.out
coverage:$(buildDir)/output.coverage
	@$(goEnv) $(gobin) tool cover -func=$< | sed -E 's%github.com/.*/poplar/%%' | column -t
coverage-html:$(buildDir)/output.coverage.html
lint:$(foreach target,$(lintPackages),$(buildDir)/output.$(target).lint)

phony += lint lint-deps build build-race race test coverage coverage-html
.PRECIOUS:$(foreach target,$(lintPackages),$(buildDir)/output.$(target).lint)
.PRECIOUS:$(buildDir)/output.lint


$(buildDir):$(srcFiles) compile
	@mkdir -p $@
$(buildDir)/output.coverage:$(buildDir) $(testFiles) .FORCE
	$(goEnv) $(gobin) test $(testArgs) -coverprofile $@ -cover $(packages)
$(buildDir)/output.coverage.html:$(buildDir)/output.coverage
	$(goEnv) $(gobin) tool cover -html=$< -o $@
#  targets to generate gotest output from the linter.
$(buildDir)/output.%.lint:$(buildDir)/run-linter $(buildDir)/ .FORCE
	@$(goEnv) ./$< --output=$@ --lintBin=$(buildDir)/golangci-lint --packages='$*'
$(buildDir)/output.lint:$(buildDir)/run-linter $(buildDir)/ .FORCE
	@$(goEnv) ./$< --output=$@ --lintBin=$(buildDir)/golangci-lint --packages='$(lintPackages)'
#  targets to process and generate coverage reports
# end test and coverage artifacts

.FORCE:


proto:vendor/cedar.proto
	@mkdir -p rpc/internal
	protoc --go_out=plugins=grpc:rpc/internal *.proto
	protoc --go_out=plugins=grpc:rpc/internal vendor/cedar.proto
	protoc --go_out=plugins=grpc:collector *.proto
	mv rpc/internal/vendor/cedar.pb.go rpc/internal/cedar.pb.go
clean:
	rm -rf $(lintDeps) rpc/internal/*.pb.go

vendor/cedar.proto:
	curl -L https://raw.githubusercontent.com/evergreen-ci/cedar/master/perf.proto -o $@
vendor:
	glide install -s


.PHONY:vendor
vendor-clean:
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/google/uuid
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/google/uuid
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/montanaflynn/stats/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/mongodb/grip/vendor/golang.org/x/sys/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/stretchr/testify
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/mongodb/mongo-go-driver/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/papertrail/
	rm -rf vendor/github.com/mongodb/grip/buildscripts/
	rm -rf vendor/github.com/mongodb/mongo-go-driver/vendor/golang.org/x/text/
	rm -rf vendor/github.com/mongodb/mongo-go-driver/vendor/golang.org/x/net/
	rm -rf vendor/github.com/mongodb/mongo-go-driver/vendor/github.com/montanaflynn/
	rm -rf vendor/github.com/mongodb/mongo-go-driver/vendor/github.com/stretchr/
	rm -rf vendor/github.com/mongodb/mongo-go-driver/vendor/github.com/google/go-cmp/cmp/
	rm -rf vendor/github.com/evergreen-ci/pail/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/pail/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/pail/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/papertrail/go-tail/vendor/golang.org/x/sys/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/mongodb/amboy/vendor/gopkg.in/yaml.v2/
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

