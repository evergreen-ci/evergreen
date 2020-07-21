buildDir := build

packages := ./ ./internal ./buildlogger ./buildlogger/fetcher ./system_metrics
lintPackages := timber buildlogger buildlogger-fetcher system_metrics
# override the go binary path if set
ifneq ($(GO_BIN_PATH),)
gobin := $(GO_BIN_PATH)
else
gobin := go
endif
gopath := $(GOPATH)
ifeq ($(OS),Windows_NT)
gopath := $(shell cygpath -m $(gopath))
endif
ifeq ($(gopath),)
gopath := $($(gobin) env GOPATH)
endif
goEnv := GOPATH=$(gopath)$(if $(GO_BIN_PATH), PATH="$(shell dirname $(GO_BIN_PATH)):$(PATH)")


# start lint setup targets
lintDeps := $(buildDir)/run-linter $(buildDir)/golangci-lint
$(buildDir)/golangci-lint:$(buildDir)
	@curl --retry 10 --retry-max-time 60 -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/76a82c6ed19784036bbf2d4c84d0228ca12381a4/install.sh | sh -s -- -b $(buildDir) v1.23.8 >/dev/null 2>&1
$(buildDir)/run-linter:cmd/run-linter/run-linter.go $(buildDir)/golangci-lint $(buildDir)
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
coverage:$(buildDir)/output.coverage
	@$(goEnv) $(gobin) tool cover -func=$< | sed -E 's%github.com/.*/jasper/%%' | column -t
coverage-html:$(buildDir)/output.coverage.html
lint:$(foreach target,$(lintPackages),$(buildDir)/output.$(target).lint)
phony += lint lint-deps build build-race race test coverage coverage-html
.PRECIOUS:$(foreach target,$(lintPackages),$(buildDir)/output.$(target).lint)
.PRECIOUS:$(buildDir)/output.lint


$(buildDir): compile
	@mkdir -p $@
$(buildDir)/output.coverage:$(buildDir) .FORCE
	$(goEnv) $(gobin) test $(testArgs) -coverprofile $@ -cover $(packages)
$(buildDir)/output.coverage.html:$(buildDir)/output.coverage
	$(goEnv) $(gobin) tool cover -html=$< -o $@
#  targets to generate gotest output from the linter.
$(buildDir)/output.%.lint:$(buildDir)/run-linter $(buildDir)/ .FORCE
	@$(goEnv) ./$< --output=$@ --lintBin=$(buildDir)/golangci-lint --packages='$*'
$(buildDir)/output.lint:$(buildDir)/run-linter $(buildDir)/ .FORCE
	@$(goEnv) ./$< --output=$@ --lintBin=$(buildDir)/golangci-lint --packages='$(packages)'
#  targets to process and generate coverage reports
# end test and coverage artifacts

.FORCE:


proto:proto-buildlogger proto-system-metrics
proto-buildlogger:buildlogger.proto
	@mkdir -p internal
	protoc --go_out=plugins=grpc:internal buildlogger.proto
	rm -rf internal/vendor
proto-system-metrics:formats.proto system_metrics.proto
	@mkdir -p internal
	protoc --go_out=plugins=grpc:internal formats.proto
	protoc --go_out=plugins=grpc:internal system_metrics.proto

clean:
	rm -rf internal/*.pb.go
	rm -f vendor/*.proto
	rm -rf $(lintDeps)

buildlogger.proto:
	curl -L https://raw.githubusercontent.com/evergreen-ci/cedar/master/buildlogger.proto -o $@
system_metrics.proto:
	curl -L https://raw.githubusercontent.com/evergreen-ci/cedar/master/system_metrics.proto -o $@
formats.proto:
	curl -L https://raw.githubusercontent.com/evergreen-ci/cedar/master/formats.proto -o $@
vendor:
	glide install -s


.PHONY:vendor
vendor-clean:
	rm -rf vendor/github.com/mongodb/grip/vendor/golang.org/x/sys/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/google.golang.org/grpc/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/github.com/pkg/errors/
	rm -rf vendor/github.com/evergreen-ci/aviation/vendor/github.com/stretchr/testify/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/stretchr/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/pkg/errors/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/golang.org/x/net/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/golang.org/x/sys/
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/golang.org/x/text/
	rm -rf vendor/go.mongodb.org/mongo-driver/data/
	find vendor/ -name "*.gif" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" -o -name "*.dat" -o -name "*testdata" | xargs rm -rf
	find vendor/ -name .git | xargs rm -rf

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
