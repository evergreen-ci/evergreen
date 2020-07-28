# start project configuration
name := gimlet
buildDir := build
packages := $(name) acl ldap okta rolemanager
orgPath := github.com/evergreen-ci
projectPath := $(orgPath)/$(name)
# end project configuration

# go environment configuration
ifneq (,$(GO_BIN_PATH))
gobin := $(GO_BIN_PATH)
else
gobin := go
endif

gopath := $(GOPATH)
ifeq ($(OS),Windows_NT)
gopath := $(shell cygpath -m $(gopath))
endif
ifeq (,$(gopath))
gopath := $($(gobin) env GOPATH)
endif
goEnv := GOPATH=$(gopath) $(if $(GO_BIN_PATH),PATH="$(shell dirname $(GO_BIN_PATH)):$(PATH)")
# end go environment configuration


# start lint setup targets
lintDeps := $(buildDir)/run-linter $(buildDir)/golangci-lint
$(buildDir)/golangci-lint:$(buildDir)
	@curl --retry 10 --retry-max-time 60 -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/76a82c6ed19784036bbf2d4c84d0228ca12381a4/install.sh | sh -s -- -b $(buildDir) v1.23.8 >/dev/null 2>&1
$(buildDir)/run-linter:buildscripts/run-linter.go $(buildDir)/golangci-lint
	@$(goEnv) $(gobin) build -o $@ $<
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
raceOutput := $(subst -,/,$(foreach target,$(packages),$(buildDir)/output.$(target).race))
coverageOutput := $(subst -,/,$(foreach target,$(packages),$(buildDir)/output.$(target).coverage))
coverageHtmlOutput := $(subst -,/,$(foreach target,$(packages),$(buildDir)/output.$(target).coverage.html))
# end dependency installation tools


# lint setup targets
# end lint setup targets

# userfacing targets for basic build and development operations
lint:$(buildDir)/output.lint
$(buildDir): $(gopath)/src/$(projectPath)
	@mkdir -p $(buildDir)
	$(goEnv) $(gobin) build ./.
build-race: $(gopath)/src/$(projectPath)
	$(goEnv) $(gobin) build -race $(subst -,/,$(foreach pkg,$(packages),./$(pkg)))
test:$(testOutput)
race:$(raceOutput)
coverage:$(coverageOutput)
coverage-html:$(coverageHtmlOutput)
phony := build build-race race test coverage coverage-html
.PRECIOUS: $(testOutput) $(raceOutput) $(coverageOutput) $(coverageHtmlOutput)
.PRECIOUS: $(buildDir)/output.lint
# end front-ends


# implementation details for building the binary and creating a
# convienent link in the working directory
$(gopath)/src/$(orgPath):
	@mkdir -p $@
$(gopath)/src/$(projectPath):$(gopath)/src/$(orgPath)
	@[ -L $@ ] || ln -s $(shell pwd) $@
$(buildDir)/$(name):$(gopath)/src/$(projectPath)
	$(goEnv) $(gobin) build -o $@ main/$(name).go
$(buildDir)/$(name).race:
	$(goEnv) $(gobin) build -race -o $@ main/$(name).go
# end main build

$(buildDir)/output.%.test:

# convenience targets for runing tests and coverage tasks on a
# specific package.
race-%:$(buildDir)/output.%.race
	@grep -s -q -e "^PASS" $< && ! grep -s -q "^WARNING: DATA RACE" $<
test-%:$(buildDir)/output.%.test
	@grep -s -q -e "^PASS" $<
coverage-%:$(buildDir)/output.%.coverage
	@grep -s -q -e "^PASS" $(subst coverage,test,$<)
html-coverage-%:$(buildDir)/output.%.coverage.html $(buildDir)/output.%.coverage.html
	@grep -s -q -e "^PASS" $(subst coverage,test,$<)
lint-%:$(buildDir)/output.%.lint
	@grep -v -s -q "^--- FAIL" $<
# end convienence targets


# start test and coverage artifacts
#    tests have compile and runtime deps. This varable has everything
#    that the tests actually need to run. (The "build" target is
#    intentional and makes these targets rerun as expected.)
testArgs := -test.v
ifneq (,$(RUN_TEST))
testArgs += -test.run='$(RUN_TEST)'
endif
ifneq (,$(RUN_CASE))
testArgs += -testify.m='$(RUN_CASE)'
endif
ifneq (,$(RUN_COUNT))
testArgs += -test.count='$(RUN_COUNT)'
endif
ifneq (,$(TEST_TIMEOUT))
testArgs += -test.timeout=$(TEST_TIMEOUT)
else
testArgs += -test.timeout=10m
endif
#    implementation for package coverage and test running,mongodb to produce
#    and save test output.
$(buildDir)/output.%.coverage.html:$(buildDir)/output.%.coverage
	$(goEnv) $(gobin) tool cover -html=$(buildDir)/output.$(subst /,-,$*).coverage -o $(buildDir)/output.$(subst /,-,$*).coverage.html
$(buildDir)/output.%.coverage: $(buildDir)
	$(goEnv) $(gobin) test $(testArgs) -covermode=count -coverprofile=$(buildDir)/output.$(subst /,-,$*).coverage $(projectPath)/$(subst -,/,$*)
	@-[ -f $(buildDir)/output.$(subst /,-,$*).coverage ] && $(goEnv) $(gobin) tool cover -func=$(buildDir)/output.$(subst /,-,$*).coverage | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/output.$(name).coverage: $(buildDir)
	$(goEnv) $(gobin) test -covermode=count -coverprofile=$@ $(projectPath)
	@-[ -f $@ ] && $(goEnv) $(gobin) tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/output.%.test: $(buildDir) .FORCE
	$(goEnv) $(gobin) test $(testArgs) ./$(subst -,/,$*) | tee $(buildDir)/output.$(subst /,-,$*).test
$(buildDir)/output.%.race: .FORCE
	$(goEnv) $(gobin) test $(testArgs) -race ./$(subst -,/,$*) | tee $(buildDir)/output.$(subst /,-,$*).race
$(buildDir)/output.$(name).test: $(buildDir) .FORCE
	$(goEnv) $(gobin) test $(testArgs) ./ | tee $@
$(buildDir)/output.$(name).race: $(buildDir) .FORCE
	$(goEnv) $(gobin) test $(testArgs) -race ./ | tee $@
#  targets to generate gotest output from the linter.
$(buildDir)/output.%.lint:$(buildDir)/run-linter .FORCE
	@$(goEnv) ./$< --output=$@ --lintBin=$(buildDir)/golangci-lint --packages='$*'
$(buildDir)/output.lint:$(buildDir)/run-linter .FORCE
	@$(goEnv) ./$< --output=$@ --lintBin=$(buildDir)/golangci-lint --packages='$(packages)'
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
	rm -rf $(name) $(lintDeps) $(buildDir)/output.*
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
