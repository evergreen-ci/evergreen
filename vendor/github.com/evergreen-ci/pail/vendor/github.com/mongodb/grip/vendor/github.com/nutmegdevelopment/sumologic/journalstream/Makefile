TARGET  ?= build

DOCKER ?= docker
DOCKERTAG ?= latest
DOCKERREPO ?= localhost
APPNAME = sumologic-journalstream
WORKDIR ?= $(CURDIR)
BUILDER ?= centurylink/golang-builder
TESTER ?= centurylink/golang-tester

build:
	$(DOCKER) build -t $(DOCKERREPO)/$(APPNAME):$(DOCKERTAG) . 
	$(DOCKER) tag -f $(DOCKERREPO)/$(APPNAME):$(DOCKERTAG) $(DOCKERREPO)/$(APPNAME):latest

test:
	go test

push:
	$(DOCKER) push $(DOCKERREPO)/$(APPNAME):$(DOCKERTAG)
	$(DOCKER) push $(DOCKERREPO)/$(APPNAME):latest

clean:
	rm -f $(APPNAME)
