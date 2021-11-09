buildDir := build
name := pail
packages := $(name)
projectPath := github.com/evergreen-ci/pail


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
$(buildDir)/golangci-lint:
	@curl --retry 10 --retry-max-time 60 -sSfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b $(buildDir) v1.30.0 >/dev/null 2>&1
$(buildDir)/run-linter:cmd/run-linter/run-linter.go $(buildDir)/golangci-lint
	@$(gobin) build -o $@ $<
# end lint setup targets


# benchmark setup targets
benchmarks:$(buildDir)/run-benchmarks .FORCE
	./$(buildDir)/run-benchmarks
$(buildDir)/run-benchmarks:cmd/run-benchmarks/run-benchmarks.go
	$(gobin) build -o $@ $<
# end benchmark setup targets


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
ifneq (,$(TEST_TIMEOUT))
	testArgs += -timeout=$(TEST_TIMEOUT)
else
	testArgs += -timeout=30m
endif

# test execution and output handlers
$(buildDir)/output.%.test:.FORCE
	$(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$*,) | tee $@
	@! grep -s -q -e "^FAIL" $@ && ! grep -s -q "^WARNING: DATA RACE" $@
#  targets to process and generate coverage reports
$(buildDir)/output.%.coverage:.FORCE
	$(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$*,) -covermode=count -coverprofile $@ | tee $(buildDir)/output.$*.test
	@-[ -f $@ ] && $(gobin) tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/output.%.coverage.html:$(buildDir)/output.%.coverage
	$(gobin) tool cover -html=$< -o $@
#  targets to generate gotest output from the linter.
# We have to handle the PATH specially for CI, because if the PATH has a different version of Go in it, it'll break.
$(buildDir)/output.%.lint: $(buildDir)/run-linter .FORCE
	@$(if $(GO_BIN_PATH), PATH="$(shell dirname $(GO_BIN_PATH)):$(PATH)") ./$< --output=$@ --lintBin=$(buildDir)/golangci-lint --packages='$*'
# end test and coverage artifacts


testOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).test)
lintOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).lint)
coverageOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage)
coverageHtmlOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage.html)


# userfacing targets for basic build and development operations
test:$(testOutput)
	
coverage:$(coverageOutput)
	
coverage-html:$(coverageHtmlOutput)
	
benchmark:
	$(gobin) test -v -benchmem -bench=. -run="Benchmark.*" -timeout=20m
lint:$(foreach target,$(packages),$(buildDir)/output.$(target).lint)

phony += lint $(buildDir) test coverage coverage-html
.PRECIOUS: $(coverageOutput) $(coverageHtmlOutput) $(lintOutput) $(testOutput)
# end front-ends


compile $(buildDir):
	$(gobin) build ./


vendor-clean:
	find vendor/ -name "*.gif" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" -o -name "*testdata*" | xargs rm -rf
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/stretchr/testify
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/pmezard
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/stretchr
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/davecgh
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/montanaflynn
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/montanaflynn
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/pkg/errors
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/mongodb/grip
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/pkg/errors
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/stretchr/testify
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/go.mongodb.org/mongo-driver
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/mongodb/ftdc/vendor/go.mongodb.org/mongo-driver/
	# Note: we have a circular dependency problem between pail and poplar
	rm -rf vendor/github.com/evergreen-ci/poplar/vendor/github.com/evergreen-ci/pail
	rm -rf vendor/github.com/evergreen-ci/utility/file.go
	rm -rf vendor/github.com/evergreen-ci/utility/http.go
	rm -rf vendor/github.com/evergreen-ci/utility/network.go
	rm -rf vendor/github.com/evergreen-ci/utility/parsing.go
	find vendor/ -type d -empty | xargs rm -rf
	find vendor/ -type d -name '.git' | xargs rm -rf
phony += vendor-clean
clean:
	rm -rf $(lintDeps)
phony += clean

# mongodb utility targets
mongodb/.get-mongodb:
	rm -rf mongodb
	mkdir -p mongodb
	cd mongodb && curl "$(MONGODB_URL)" -o mongodb.tgz && $(DECOMPRESS) mongodb.tgz && chmod +x ./mongodb-*/bin/*
	cd mongodb && mv ./mongodb-*/bin/* . && rm -rf db_files && rm -rf db_logs && mkdir -p db_files && mkdir -p db_logs
get-mongodb: mongodb/.get-mongodb
	@touch $<
start-mongod: mongodb/.get-mongodb
	./mongodb/mongod --dbpath ./mongodb/db_files
	@echo "waiting for mongod to start up"
check-mongod: mongodb/.get-mongodb
	./mongodb/mongo --nodb --eval "assert.soon(function(x){try{var d = new Mongo(\"localhost:27017\"); return true}catch(e){return false}}, \"timed out connecting\")"
	@echo "mongod is up"
# end mongodb targets

# configure phony targets
.FORCE:
.PHONY:$(phony) .FORCE
