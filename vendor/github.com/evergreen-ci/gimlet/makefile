# start project configuration
name := gimlet
buildDir := build
packages := $(name) acl ldap okta rolemanager
orgPath := github.com/evergreen-ci
projectPath := $(orgPath)/$(name)
# end project configuration

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


######################################################################
##
## Everything below this point is generic, and does not contain
## project specific configuration. (with one noted case in the "build"
## target for library-only projects)
##
######################################################################


# start dependency installation tools
#   implementation details for being able to lazily install dependencies
testOutput := $(subst -,/,$(foreach target,$(packages),$(buildDir)/output.$(target).test))
lintOutput := $(subst -,/,$(foreach target,$(packages),$(buildDir)/output.$(target).lint))
coverageOutput := $(subst -,/,$(foreach target,$(packages),$(buildDir)/output.$(target).coverage))
coverageHtmlOutput := $(subst -,/,$(foreach target,$(packages),$(buildDir)/output.$(target).coverage.html))
# end dependency installation tools


# lint setup targets
# end lint setup targets

# userfacing targets for basic build and development operations
lint:$(lintOutput)
compile $(buildDir):
	$(gobin) build ./.
test:$(testOutput)
coverage:$(coverageOutput)
coverage-html:$(coverageHtmlOutput)
phony := build test coverage coverage-html
.PRECIOUS: $(testOutput) $(lintOuptut) $(coverageOutput) $(coverageHtmlOutput)
# end front-ends

# convenience targets for runing tests and coverage tasks on a
# specific package.
test-%:$(buildDir)/output.%.test
	@grep -s -q -e "^PASS" $<
coverage-%:$(buildDir)/output.%.coverage

html-coverage-%:$(buildDir)/output.%.coverage.html
	
lint-%:$(buildDir)/output.%.lint
	@grep -v -s -q "^--- FAIL" $<
# end convienence targets


# start test and coverage artifacts
#    tests have compile and runtime deps. This varable has everything
#    that the tests actually need to run. (The "build" target is
#    intentional and makes these targets rerun as expected.)
testArgs := -v
ifeq (,$(DISABLE_COVERAGE))
	testArgs += -cover
endif
ifneq (,$(RACE_DETECTOR))
	testArgs += -race
endif
ifneq (,$(RUN_TEST))
	testArgs += -run='$(RUN_TEST)'
endif
ifneq (,$(RUN_COUNT))
	testArgs += -count=$(RUN_COUNT)
endif
ifneq (,$(TEST_TIMEOUT))
	testArgs += -timeout=$(TEST_TIMEOUT)
endif
#    implementation for package coverage and test running,mongodb to produce
#    and save test output.
$(buildDir)/output.%.coverage: .FORCE
	$(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) -covermode=count -coverprofile $@ | tee $(buildDir)/output.$*.test
	@-[ -f $@ ] && $(gobin) tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/output.%.coverage.html: $(buildDir)/output.%.coverage .FORCE
	$(gobin) tool cover -html=$< -o $@
$(buildDir)/output.%.test: .FORCE
	$(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) | tee $(buildDir)/output.$(subst /,-,$*).test
#  targets to generate gotest output from the linter.
# We have to handle the PATH specially for CI, because if the PATH has a different version of Go in it, it'll break.
$(buildDir)/output.%.lint: $(buildDir)/run-linter .FORCE
	@$(if $(GO_BIN_PATH), PATH="$(shell dirname $(GO_BIN_PATH)):$(PATH)") ./$< --output=$@ --lintBin=$(buildDir)/golangci-lint --packages='$*'
# end test and coverage artifacts


# start vendoring configuration
vendor-clean:
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/pkg/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/stretchr/
	rm -rf vendor/gopkg.in/asn1-ber.v1/tests/
	rm -rf vendor/github.com/rs/cors/examples/
	find vendor/ -name "*.gif" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" -o -name "*.dat" -o -name "*testdata" | xargs rm -rf
	find vendor/ -type d -name '.git' | xargs rm -rf
phony += vendor-clean
# end vendoring tooling configuration


# clean and other utility targets
clean:
	rm -rf $(lintDeps)
clean-results:
	rm -rf $(buildDir)/output.*
phony += clean
# end dependency targets

# mongodb utility targets
mongodb/.get-mongodb:
	rm -rf mongodb
	mkdir -p mongodb
	cd mongodb && curl "$(MONGODB_URL)" -o mongodb.tgz && $(DECOMPRESS) mongodb.tgz && chmod +x ./mongodb-*/bin/*
	cd mongodb && mv ./mongodb-*/bin/* . && rm -rf db_files && rm -rf db_logs && mkdir -p db_files && mkdir -p db_logs
get-mongodb: mongodb/.get-mongodb
	@touch $<
start-mongod: mongodb/.get-mongodb
	./mongodb/mongod --dbpath ./mongodb/db_files --port 27017 --replSet evg --smallfiles --oplogSize 10
	@echo "waiting for mongod to start up"
init-rs:mongodb/.get-mongodb
	./mongodb/mongo --eval 'rs.initiate()'
check-mongod: mongodb/.get-mongodb
	./mongodb/mongo --nodb --eval "assert.soon(function(x){try{var d = new Mongo(\"localhost:27017\"); return true}catch(e){return false}}, \"timed out connecting\")"
	@echo "mongod is up"
# end mongodb targets

# configure phony targets
.FORCE:
.PHONY:$(phony)
