SUBDIRS:=ctriface taps misc
EXTRAGOARGS:=-v -race -cover
EXTRAGOARGS_NORACE:=-v
EXTRATESTFILES:=fccd-orchestrator_test.go stats.go fccd-orchestrator.go functions.go 

fccd: proto
	go install github.com/ustiugov/fccd-orchestrator

protobuf:
	protoc -I proto/ proto/orchestrator.proto --go_out=plugins=grpc:proto
	protoc -I ../workloads/protos ../workloads/protos/helloworld.proto --go_out=plugins=grpc:helloworld

clean:
	rm proto/orchestrator.pb.go

test-all: test $(SUBDIRS) test-man

test:
	sudo env "PATH=$(PATH)" go test $(EXTRATESTFILES) $(EXTRAGOARGS)

test-man:
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS_NORACE) -run TestParallelLoadServe
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestLoadServeMultiple1
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestLoadServeMultiple2
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestLoadServeMultiple3
	./scripts/clean_fcctr.sh

$(SUBDIRS):
	$(MAKE) -C $@ test
	$(MAKE) -C $@ test-man

.PHONY: test-all $(SUBDIRS)
