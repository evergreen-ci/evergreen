name := timber
buildDir := build

packages := $(name) buildlogger buildlogger-fetcher system_metrics
testPackages := buildlogger buildlogger-fetcher system_metrics

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
lintDeps := $(buildDir)/run-linter $(buildDir)/golangci-lint
$(buildDir)/golangci-lint:$(buildDir)
	@curl --retry 10 --retry-max-time 60 -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/76a82c6ed19784036bbf2d4c84d0228ca12381a4/install.sh | sh -s -- -b $(buildDir) v1.30.0 >/dev/null 2>&1
$(buildDir)/run-linter:cmd/run-linter/run-linter.go $(buildDir)/golangci-lint $(buildDir)
	@$(gobin) build -o $@ $<
# end lint setup targets

testOutput := $(foreach target,$(testPackages),$(buildDir)/output.$(target).test)
lintOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).lint)
coverageOutput := $(foreach target,$(testPackages),$(buildDir)/output.$(target).coverage)
coverageHtmlOutput := $(foreach target,$(testPackages),$(buildDir)/output.$(target).coverage.html)

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

test:$(testOutput)
	
coverage:$(coverageOutput)
	@$(gobin) tool cover -func=$< | sed -E 's%github.com/.*/jasper/%%' | column -t
coverage-html:$(coverageHtmlOutput)
	
lint:$(lintOutput)
	
phony += lint $(buildDir) race test coverage coverage-html
.PRECIOUS:$(coverageOutput) $(coverageHtmlOutput) $(lintOutput) $(testOutput)


compile $(buildDir):
	$(gobin) build $(subst $(name),,$(subst -,/,$(foreach target,$(packages),./$(target))))
# test execution and output handlers
$(buildDir)/output.%.test: .FORCE
	$(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) | tee $@
	@!( grep -s -q "^FAIL" $@ && grep -s -q "^WARNING: DATA RACE" $@)
	@(grep -s -q "^PASS" $@ || grep -s -q "no test files" $@)
#  targets to process and generate coverage reports
$(buildDir)/output.%.coverage:.FORCE
	$(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) -covermode=count -coverprofile $@ | tee $(buildDir)/output.$*.test
	@-[ -f $@ ] && $(gobin) tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/output.%.coverage.html:$(buildDir)/output.%.coverage
	$(gobin) tool cover -html=$< -o $@
#  targets to generate gotest output from the linter.
# We have to handle the PATH specially for CI, because if the PATH has a different version of Go in it, it'll break.
$(buildDir)/output.%.lint: $(buildDir)/run-linter .FORCE
	@$(if $(GO_BIN_PATH), PATH="$(shell dirname $(GO_BIN_PATH)):$(PATH)") ./$< --output=$@ --lintBin=$(buildDir)/golangci-lint --packages='$*'
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
clean-results:
	rm -rf $(buildDir)/output.*

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
	
coverage-%:$(buildDir)/output.%.coverage
	@grep -s -q -e "^PASS" $(buildDir)/output.$*.test
html-coverage-%:$(buildDir)/output.%.coverage.html
	@grep -s -q -e "^PASS" $(buildDir)/output.$*.test
lint-%:$(buildDir)/output.%.lint
	@grep -v -s -q "^--- FAIL" $<
# end convienence targets
