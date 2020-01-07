buildDir := build
srcFiles := $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -name "*_test.go" -not -path "*\#*")
testFiles := $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -path "*\#*")
projectPath := github.com/evergreen-ci/pail

packages := pail

# start environment setup
ifneq (,${GO_BIN_PATH})
gobin := ${GO_BIN_PATH}
else
gobin := go
endif
gopath := $(GOPATH)
ifeq ($(OS),Windows_NT)
gopath := $(shell cygpath -m $(gopath))
userProfile := $(shell cygpath -m $(USERPROFILE))
endif
goEnv := GOPATH=$(gopath) $(if ${GO_BIN_PATH},PATH="$(shell dirname ${GO_BIN_PATH}):${PATH}")
# end environment setup

# start linting configuration
#   package, testing, and linter dependencies specified
#   separately. This is a temporary solution: eventually we should
#   vendorize all of these dependencies.
lintDeps := github.com/alecthomas/gometalinter
#   include test files and give linters 40s to run to avoid timeouts
lintArgs := --tests --deadline=13m --vendor
#   gotype produces false positives because it reads .a files which
#   are rarely up to date.
lintArgs += --disable="gotype" --disable="gosec" --disable="gocyclo" --enable="golint"
lintArgs += --skip="build"
#   enable and configure additional linters
lintArgs += --line-length=100 --dupl-threshold=150
#   some test cases are structurally similar, and lead to dupl linter
#   warnings, but are important to maintain separately, and would be
#   difficult to test without a much more complex reflection/code
#   generation approach, so we ignore dupl errors in tests.
lintArgs += --exclude="warning: duplicate of .*_test.go"
#   go lint warns on an error in docstring format, erroneously because
#   it doesn't consider the entire package.
lintArgs += --exclude="warning: package comment should be of the form \"Package .* ...\""
#   known issues that the linter picks up that are not relevant in our cases
lintArgs += --exclude="file is not goimported" # top-level mains aren't imported
lintArgs += --exclude="error return value not checked .defer.*"
# end linting configuration


# start dependency installation tools
#   implementation details for being able to lazily install dependencies
lintDeps := $(addprefix $(gopath)/src/,$(lintDeps))
$(gopath)/src/%:
	@-[ ! -d $(gopath) ] && mkdir -p $(gopath) || true
	$(goEnv) $(gobin) get $(subst $(gopath)/src/,,$@)
$(buildDir)/run-linter:cmd/run-linter/run-linter.go $(buildDir)/.lintSetup
	 $(goEnv) $(gobin) build -o $@ $<
$(buildDir)/.lintSetup:$(lintDeps)
	@-$(gopath)/bin/gometalinter --install >/dev/null && touch $@
# end dependency installation tools


testArgs := -v
ifneq (,$(RUN_TEST))
testArgs += -run='$(RUN_TEST)'
endif
ifneq (,$(RUN_COUNT))
  WORK_DIR: ${workdir}
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
ifneq (,$(TEST_TIMEOUT))
testArgs += -timeout=$(TEST_TIMEOUT)
else
testArgs += -timeout=30m
endif

# test execution and output handlers
$(buildDir)/:
	mkdir -p $@
$(buildDir)/output.%.test:$(buildDir)/ .FORCE
	export USERPROFILE=$(userProfile)
	$(goEnv) $(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$*,) | tee $@
	@! grep -s -q -e "^FAIL" $@ && ! grep -s -q "^WARNING: DATA RACE" $@
$(buildDir)/output.test:$(buildDir)/ .FORCE
	export USERPROFILE=$(userProfile)
	$(goEnv) $(gobin) test $(testArgs) ./... | tee $@
	@! grep -s -q -e "^FAIL" $@ && ! grep -s -q "^WARNING: DATA RACE" $@
$(buildDir)/output.%.coverage:$(buildDir)/ .FORCE
	export USERPROFILE=$(userProfile)
	$(goEnv) $(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$*,) -covermode=count -coverprofile $@ | tee $(buildDir)/output.$*.test
	@-[ -f $@ ] && $(goEnv) $(gobin) tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/output.%.coverage.html:$(buildDir)/output.%.coverage
	$(goEnv) $(gobin) tool cover -html=$< -o $@
#  targets to generate gotest output from the linter.
$(buildDir)/output.%.lint:$(buildDir)/run-linter $(buildDir)/ .FORCE
	@./$< --output=$@ --lintArgs='$(lintArgs)' --packages='$*'
$(buildDir)/output.lint:$(buildDir)/run-linter $(buildDir)/ .FORCE
	@./$< --output="$@" --lintArgs='$(lintArgs)' --packages="$(packages)"
#  targets to process and generate coverage reports
# end test and coverage artifacts


# userfacing targets for basic build and development operations
compile:
	$(goEnv) $(gobin) build ./
test:$(buildDir)/test.out
$(buildDir)/test.out:.FORCE
	export USERPROFILE=$(userProfile)
	@mkdir -p $(buildDir)
	$(goEnv) $(gobin) test $(testArgs) ./ | tee $@
	@grep -s -q -e "^PASS" $@
coverage:$(buildDir)/cover.out
	@$(goEnv) $(gobin) tool cover -func=$< | sed -E 's%$(projectPath)/%%' | column -t
coverage-html:$(buildDir)/cover.html

benchmark:
	$(goEnv) $(gobin) test -v -benchmem -bench=. -run="Benchmark.*" -timeout=20m
lint:$(foreach target,$(packages),$(buildDir)/output.$(target).lint)

phony += lint lint-deps build build-race race test coverage coverage-html
.PRECIOUS:$(foreach target,$(packages),$(buildDir)/output.$(target).lint)
.PRECIOUS:$(buildDir)/output.lint
# end front-ends


$(buildDir):$(srcFiles) compile
	@mkdir -p $@
$(buildDir)/cover.out:$(buildDir) $(testFiles) .FORCE
	export USERPROFILE=$(userProfile)
	$(goEnv) $(gobin) test $(testArgs) -covermode=count -coverprofile $@ -cover ./
$(buildDir)/cover.html:$(buildDir)/cover.out
	$(goEnv) $(gobin) tool cover -html=$< -o $@


vendor-clean:
	find vendor/ -name "*.gif" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" -o -name "*testdata*" | xargs rm -rf
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/stretchr/testify
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/pmezard
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/stretchr
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/davecgh
	rm -rf vendor/go.mongodb.org/mongo-driver/vendor/github.com/montanaflynn
phony += vendor-clean

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
