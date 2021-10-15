# start project configuration
name := amboy
buildDir := build
packages := $(name) dependency job registry pool queue rest logger management cli
compilePackages := $(subst $(name),,$(subst -,/,$(foreach target,$(packages),./$(target))))
orgPath := github.com/mongodb
projectPath := $(orgPath)/$(name)
# end project configuration

# start environment setup
gobin := go
ifneq ($(GOROOT),)
gobin := $(GOROOT)/bin/go
endif

ifeq ($(OS),Windows_NT)
gobin := $(shell cygpath $(gobin))
export GOCACHE := $(shell cygpath -m $(abspath $(buildDir)/.cache))
export GOLANGCI_LINT_CACHE := $(shell cygpath -m $(abspath $(buildDir)/.lint-cache))
export GOPATH := $(shell cygpath -m $(GOPATH))
export GOROOT := $(shell cygpath -m $(GOROOT))
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
lintDeps := $(buildDir)/run-linter $(buildDir)/golangci-lint
$(buildDir)/golangci-lint:
	@curl --retry 10 --retry-max-time 60 -sSfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b $(buildDir) v1.40.0 >/dev/null 2>&1
$(buildDir)/run-linter: cmd/run-linter/run-linter.go $(buildDir)/golangci-lint
	@$(gobin) build -o $@ $<
# end lint setup targets

# start benchmark targets
benchmarks: $(buildDir)/run-benchmarks .FORCE
	./$(buildDir)/run-benchmarks $(run-benchmark)
$(buildDir)/run-benchmarks: cmd/run-benchmarks/run-benchmarks.go
	$(gobin) build -o $@ $<
# end benchmark targets

# start output files
testOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).test)
testOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).lint)
coverageOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage)
coverageHtmlOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage.html)
.PRECIOUS: $(testOutput) $(lintOutput) $(coverageOutput) $(coverageHtmlOutput)
# end output files

# start basic development targets
compile:
	$(gobin) build $(compilePackages)
test: $(foreach target,$(packages),$(buildDir)/output.$(target).test)
lint: $(foreach target,$(packages),$(buildDir)/output.$(target).lint)
coverage: $(coverageOutput)
coverage-html: $(coverageHtmlOutput)
phony := compile lint test coverage coverage-html

# start convenience targets for running tests and coverage tasks on a
# specific package.
test-%: $(buildDir)/output.%.test
	
coverage-%: $(buildDir)/output.%.coverage
	
html-coverage-%: $(buildDir)/output.%.coverage.html
	
lint-%: $(buildDir)/output.%.lint
	
# end convenience targets
# end basic development targets

# start test and coverage artifacts
testArgs := -v -timeout=1h
ifneq (,$(RUN_TEST))
testArgs += -run='$(RUN_TEST)'
endif
ifneq (,$(RUN_COUNT))
testArgs += -count='$(RUN_COUNT)'
endif
ifneq (,$(RACE_DETECTOR))
testArgs += -race
endif
ifeq (,$(DISABLE_COVERAGE))
testArgs += -cover
endif
ifneq (,$(SKIP_LONG))
testArgs += -short
endif
$(buildDir)/output.%.test: .FORCE
	$(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) | tee $@
	@!( grep -s -q "^FAIL" $@ && grep -s -q "^WARNING: DATA RACE" $@)
	@(grep -s -q "^PASS" $@ || grep -s -q "no test files" $@)
$(buildDir)/output.%.coverage: .FORCE
	$(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) -coverprofile $@ | tee $(buildDir)/output.$*.test
	@-[ -f $@ ] && $(gobin) tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
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

# start mongodb targets
mongodb/.get-mongodb:
	rm -rf mongodb
	mkdir -p mongodb
	cd mongodb && curl "$(MONGODB_URL)" -o mongodb.tgz && $(DECOMPRESS) mongodb.tgz && chmod +x ./mongodb-*/bin/*
	cd mongodb && mv ./mongodb-*/bin/* . && rm -rf db_files && rm -rf db_logs && mkdir -p db_files && mkdir -p db_logs
get-mongodb: mongodb/.get-mongodb
	@touch $<
start-mongod: mongodb/.get-mongodb
	./mongodb/mongod --dbpath ./mongodb/db_files --port 27017 --replSet amboy
	@echo "waiting for mongod to start up"
check-mongod: mongodb/.get-mongodb
	./mongodb/mongo --nodb --eval "assert.soon(function(x){try{var d = new Mongo(\"localhost:27017\"); return true}catch(e){return false}}, \"timed out connecting\")"
	@echo "mongod is up"
init-rs: mongodb/.get-mongodb
	./mongodb/mongo --eval 'rs.initiate()'
phony += get-mongodb start-mongod check-mongod init-rs
# end mongodb targets

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
	rm -rf vendor/github.com/evergreen-ci/utility/file.go
	rm -rf vendor/github.com/evergreen-ci/utility/gitignore.go
	rm -rf vendor/github.com/evergreen-ci/utility/http.go
	rm -rf vendor/github.com/evergreen-ci/utility/network.go
	rm -rf vendor/github.com/evergreen-ci/utility/parsing.go
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
phony += vendor-clean
# end vendoring configuration


# start cleanup targets
clean:
	rm -rf $(buildDir)
clean-results:
	rm -rf $(buildDir)/output.*
phony += clean clean-results
# end cleanup targets

# configure phony targets
.FORCE:
.PHONY: $(phony)
