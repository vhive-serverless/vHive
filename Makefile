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

test-orch: test test-man

test:
	sudo env "PATH=$(PATH)" "GOORCHSNAPSHOTS=false" go test $(EXTRATESTFILES) $(EXTRAGOARGS)
	sudo env "PATH=$(PATH)" "GOORCHSNAPSHOTS=true" go test $(EXTRATESTFILES) $(EXTRAGOARGS)

test-man:
	sudo env "PATH=$(PATH)" "GOORCHSNAPSHOTS=false" go test $(EXTRAGOARGS_NORACE) -run TestParallelServe
	sudo env "PATH=$(PATH)" "GOORCHSNAPSHOTS=false" go test $(EXTRAGOARGS) -run TestServeThree
	sudo env "PATH=$(PATH)" "GOORCHSNAPSHOTS=true" go test $(EXTRAGOARGS_NORACE) -run TestParallelServe
	sudo env "PATH=$(PATH)" "GOORCHSNAPSHOTS=true" go test $(EXTRAGOARGS) -run TestServeThree
	./scripts/clean_fcctr.sh

$(SUBDIRS):
	$(MAKE) -C $@ test
	$(MAKE) -C $@ test-man

test-subdirs: $(SUBDIRS)

.PHONY: test-orch $(SUBDIRS) test-subdirs
