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


# start linting configuration
#   package, testing, and linter dependencies specified
#   separately. This is a temporary solution: eventually we should
#   vendorize all of these dependencies.
lintDeps := github.com/alecthomas/gometalinter
lintDeps += github.com/richardsamuels/evg-lint/...
#   include test files and give linters 40s to run to avoid timeouts
lintArgs := --tests --deadline=1m --vendor --aggregate --sort=line
#   gotype produces false positives because it reads .a files which
#   are rarely up to date.
lintArgs += --disable="gotype" --disable="golint" --disable="gosec" --disable="staticcheck"
lintArgs += --enable="goimports"
lintArgs += --vendored-linters --enable-gc
#  add and configure additional linters
lintArgs += --line-length=100 --dupl-threshold=175 --cyclo-over=17
#  two similar functions triggered the duplicate warning, but they're not.
lintArgs += --exclude="duplicate of registry.go"
lintArgs += --exclude="don.t use underscores.*_DependencyState.*"
#  golint doesn't handle splitting package comments between multiple files.
lintArgs += --exclude="package comment should be of the form \"Package .* \(golint\)"
lintArgs += --exclude="error return value not checked \(defer .* \(errcheck\)$$"
lintArgs += --exclude="declaration of \"assert\" shadows declaration at .*_test.go:"
lintArgs += --exclude="declaration of \"require\" shadows declaration at .*_test.go:"
lintArgs += --linter="evg:$(gopath)/bin/evg-lint:PATH:LINE:COL:MESSAGE" --enable=evg
# end lint suppressions

######################################################################
##
## Everything below this point is generic, and does not contain
## project specific configuration. (with one noted case in the "build"
## target for library-only projects)
##
######################################################################


# start dependency installation tools
#   implementation details for being able to lazily install dependencies
lintDeps := $(addprefix $(gopath)/src/,$(lintDeps))
srcFiles := makefile $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -name "*_test.go")
testSrcFiles := makefile $(shell find . -name "*.go" -not -path "./$(buildDir)/*")
testOutput := $(subst -,/,$(foreach target,$(packages),$(buildDir)/output.$(target).test))
raceOutput := $(subst -,/,$(foreach target,$(packages),$(buildDir)/output.$(target).race))
coverageOutput := $(subst -,/,$(foreach target,$(packages),$(buildDir)/output.$(target).coverage))
coverageHtmlOutput := $(subst -,/,$(foreach target,$(packages),$(buildDir)/output.$(target).coverage.html))
$(gopath)/src/%:
	@-[ ! -d $(gopath) ] && mkdir -p $(gopath) || true
	$(goEnv) $(gobin) get $(subst $(gopath)/src/,,$@)
# end dependency installation tools


# lint setup targets
lintDeps := $(addprefix $(gopath)/src/,$(lintDeps))
lintTargets := $(foreach target,$(packages),lint-$(target))
$(buildDir)/.lintSetup:$(lintDeps)
	@mkdir -p $(buildDir)
	@-$(goEnv) $(gopath)/bin/gometalinter --force --install >/dev/null && touch $@
	@touch $@
$(buildDir)/run-linter:buildscripts/run-linter.go
	$(goEnv) $(gobin) build -o $@ $<
.PRECIOUS:$(buildDir)/output.lint
# end lint setup targets

# userfacing targets for basic build and development operations
lint:$(buildDir)/output.lint
lint-deps:$(lintDeps)
build:$(deps) $(srcFiles) $(gopath)/src/$(projectPath)
	@mkdir -p $(buildDir)
	$(goEnv) $(gobin) build ./.
build-race:$(deps) $(srcFiles) $(gopath)/src/$(projectPath)
	$(goEnv) $(gobin) build -race $(subst -,/,$(foreach pkg,$(packages),./$(pkg)))
test:$(testOutput)
race:$(raceOutput)
coverage:$(coverageOutput)
coverage-html:$(coverageHtmlOutput)
phony := build build-race race test coverage coverage-html
phony += deps test-deps lint-deps
.PRECIOUS: $(testOutput) $(raceOutput) $(coverageOutput) $(coverageHtmlOutput)
# end front-ends


# implementation details for building the binary and creating a
# convienent link in the working directory
$(gopath)/src/$(orgPath):
	@mkdir -p $@
$(gopath)/src/$(projectPath):$(gopath)/src/$(orgPath)
	@[ -L $@ ] || ln -s $(shell pwd) $@
$(buildDir)/$(name):$(gopath)/src/$(projectPath) $(srcFiles) $(deps)
	$(goEnv) $(gobin) build -o $@ main/$(name).go
$(buildDir)/$(name).race:$(gopath)/src/$(projectPath) $(srcFiles) $(deps)
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
testRunDeps := $(testSrcFiles) build
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
$(buildDir)/output.%.coverage:$(testRunDeps)
	$(goEnv) $(gobin) test $(testArgs) -covermode=count -coverprofile=$(buildDir)/output.$(subst /,-,$*).coverage $(projectPath)/$(subst -,/,$*)
	@-[ -f $(buildDir)/output.$(subst /,-,$*).coverage ] && $(goEnv) $(gobin) tool cover -func=$(buildDir)/output.$(subst /,-,$*).coverage | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/output.$(name).coverage:$(testRunDeps)
	$(goEnv) $(gobin) test -covermode=count -coverprofile=$@ $(projectPath)
	@-[ -f $@ ] && $(goEnv) $(gobin) tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/output.%.test:$(testRunDeps) .FORCE
	$(goEnv) $(gobin) test $(testArgs) ./$(subst -,/,$*) | tee $(buildDir)/output.$(subst /,-,$*).test
$(buildDir)/output.%.race:$(testRunDeps) .FORCE
	$(goEnv) $(gobin) test $(testArgs) -race ./$(subst -,/,$*) | tee $(buildDir)/output.$(subst /,-,$*).race
$(buildDir)/output.$(name).test:$(testRunDeps) .FORCE
	$(goEnv) $(gobin) test $(testArgs) ./ | tee $@
$(buildDir)/output.$(name).race:$(testRunDeps) .FORCE
	$(goEnv) $(gobin) test $(testArgs) -race ./ | tee $@
#  targets to generate gotest output from the linter.
$(buildDir)/output.%.lint:$(buildDir)/run-linter $(testSrcFiles)  $(buildDir)/.lintSetup .FORCE
	@./$< --output=$@ --lintArgs='$(lintArgs)' --packages='$*'
$(buildDir)/output.lint:$(buildDir)/run-linter $(buildDir)/.lintSetup .FORCE
	@./$< --output="$@" --lintArgs='$(lintArgs)' --packages="$(packages)"
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
