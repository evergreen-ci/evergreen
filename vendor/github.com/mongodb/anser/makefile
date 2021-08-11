# start project configuration
name := anser
buildDir := build
packages := $(name) mock model db bsonutil client apm backup
orgPath := github.com/mongodb
projectPath := $(orgPath)/$(name)
# end project configuration

# start environment setup
gobin := $(GO_BIN_PATH)
ifeq (,$(gobin))
	gobin := go
endif

gocache := $(abspath $(buildDir)/.cache)
gopath := $(GOPATH)
goroot := $(goroot)
ifeq (,$(gopath))
gopath := $(shell $(gobin) env GOPATH)
endif
ifeq ($(OS),Windows_NT)
gocache := $(shell cygpath -m $(gocache))
gopath := $(shell cygpath -m $(gopath))
goroot := $(shell cygpath -m $(goroot))
endif
export GOCACHE := $(gocache)
export GOPATH := $(gopath)
export GOROOT := $(goroot)
export GO111MODULE := off
# end environment setup

# Ensure the build directory exists, since most targets require it.
$(shell mkdir -p $(buildDir))

# start dependency installation tools
#   implementation details for being able to lazily install dependencies.
#   this block has no project specific configuration but defines
#   variables that project specific information depends on
testOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).test)
raceOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).race)
testBin := $(foreach target,$(packages),$(buildDir)/test.$(target))
raceBin := $(foreach target,$(packages),$(buildDir)/race.$(target))
lintTargets := $(foreach target,$(packages),lint-$(target))
coverageOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage)
coverageHtmlOutput := $(foreach target,$(packages),$(buildDir)/output.$(target).coverage.html)
testSrcFiles := makefile $(shell find . -name "*.go" -not -path "./$(buildDir)/*" -not -path "*\#*")
# end dependency installation tools

# lint setup targets
lintDeps := $(buildDir)/golangci-lint $(buildDir)/run-linter
$(buildDir)/golangci-lint:
	@curl --retry 10 --retry-max-time 60 -sSfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b $(buildDir) v1.40.0 >/dev/null 2>&1
$(buildDir)/run-linter:cmd/run-linter/run-linter.go $(buildDir)/golangci-lint
	$(gobin) build -o $@ $<
# end lint setup targets


# userfacing targets for basic build and development operations
compile $(buildDir):
	$(gobin) build $(subst $(name),,$(subst -,/,$(foreach pkg,$(packages),./$(pkg))))
test:$(testOutput)
coverage:$(coverageOutput)
coverage-html:$(coverageHtmlOutput)
lint:$(lintTargets)
list-tests:
	@echo -e "test targets:" $(foreach target,$(packages),\\n\\ttest-$(target))
phony := lint build test coverage coverage-html
.PRECIOUS:$(testOutput) $(raceOutput) $(coverageOutput) $(coverageHtmlOutput)
.PRECIOUS:$(foreach target,$(packages),$(buildDir)/test.$(target))
.PRECIOUS:$(foreach target,$(packages),$(buildDir)/output.$(target).lint)
# end front-ends


# convenience targets for runing tests and coverage tasks on a
# specific package.
race-%:$(buildDir)/output.%.race
	@grep -s -q -e "^PASS" $< && ! grep -s -q "^WARNING: DATA RACE" $<
test-%:$(buildDir)/output.%.test
	@grep -s -q -e "^PASS" $<
coverage-%:$(buildDir)/output.%.coverage
	@grep -s -q -e "^PASS" $(subst coverage,test,$<)
html-coverage-%:$(buildDir)/output.%.coverage $(buildDir)/output.%.coverage.html
	@grep -s -q -e "^PASS" $(subst coverage,test,$<)
lint-%:$(buildDir)/output.%.lint
	@grep -v -s -q "^--- FAIL" $<
# end convienence targets


# start vendoring configuration
vendor-clean:
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/mongodb/amboy/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/mongodb/amboy/vendor/go.mongodb.org/mongo-driver/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/evergreen-ci/birch/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/mongodb/grip/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/pkg/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/go.mongodb.org/mongo-driver/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/gopkg.in/mgo.v2/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/gopkg.in/mgo.v2/
	rm -rf vendor/github.com/mongodb/grip/vendor/github.com/stretchr/testify/
	rm -rf vendor/github.com/mongodb/ftdc/vendor/github.com/satori/go.uuid/
	rm -rf vendor/gopkg.in/mgo.v2/harness/
	find vendor/ -name "*.gif" -o -name "*.gz" -o -name "*.png" -o -name "*.ico" -o -name "*.dat" -o -name "*testdata" | xargs rm -rf
	find vendor/ -name '.git' | xargs rm -rf
#   add phony targets
phony += vendor-clean
# end vendoring tooling configuration


# start test and coverage artifacts
#    This varable includes everything that the tests actually need to
#    run. (The "build" target is intentional and makes these targetsb
#    rerun as expected.)
testArgs := -v -timeout=10m
ifneq (,$(RUN_TEST))
testArgs += -run='$(RUN_TEST)'
endif
ifneq (,$(RUN_COUNT))
testArgs += -count=$(RUN_COUNT)
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
# testing targets
$(buildDir)/output.%.test: .FORCE
	$(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) | tee $@
$(buildDir)/output.%.coverage: .FORCE
	$(gobin) test $(testArgs) ./$(if $(subst $(name),,$*),$(subst -,/,$*),) -covermode=count -coverprofile $@ | tee $(buildDir)/output.$*.test
	@-[ -f $@ ] && $(gobin) tool cover -func=$@ | sed 's%$(projectPath)/%%' | column -t
$(buildDir)/output.%.coverage.html:$(buildDir)/output.%.coverage
	$(gobin) tool cover -html=$< -o $@
#  targets to generate gotest output from the linter.
# We have to handle the PATH specially for CI, because if the PATH has a different version of Go in it, it'll break.
$(buildDir)/output.%.lint:$(buildDir)/run-linter .FORCE
	@$(if $(GO_BIN_PATH),PATH="$(shell dirname $(GO_BIN_PATH)):$(PATH)") ./$< --output=$@ --lintBin="$(buildDir)/golangci-lint" --packages='$*'
# end test and coverage artifacts

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
init-rs: mongodb/.get-mongodb
	./mongodb/mongo --eval 'rs.initiate()'
check-mongod: mongodb/.get-mongodb
	./mongodb/mongo --nodb --eval "assert.soon(function(x){try{var d = new Mongo(\"localhost:27017\"); return true}catch(e){return false}}, \"timed out connecting\")"
	@echo "mongod is up"
# end mongodb targets


# clean and other utility targets
clean:
	rm -rf $(lintDeps)
clean-results:
	rm -rf $(buildDir)/output.*
phony += clean
# end dependency targets

# configure phony targets
.FORCE:
.PHONY:$(phony) .FORCE
