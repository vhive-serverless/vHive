SUBDIRS:=ctriface taps misc
EXTRAGOARGS:=-v -race -cover
EXTRAGOARGS_NORACE:=-v
EXTRATESTFILES:=fccd-orchestrator_test.go stats.go fccd-orchestrator.go functions.go
WITHUPF:=-upfTest
WITHLAZY:=-lazyTest
WITHSNAPSHOTS:=-snapshotsTest
CTRDLOGDIR:=/tmp/ctrd-logs

fccd: proto
	go install github.com/ustiugov/fccd-orchestrator

protobuf:
	protoc -I proto/ proto/orchestrator.proto --go_out=plugins=grpc:proto
	protoc -I ../workloads/protos ../workloads/protos/helloworld.proto --go_out=plugins=grpc:helloworld

clean:
	rm proto/orchestrator.pb.go

test-all: test-subdirs test-orch

test-orch: test test-man

test:
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/fccd_orch_noupf_log.out 2>$(CTRDLOGDIR)/fccd_orch_noupf_log.err &
	sudo env "PATH=$(PATH)" go test $(EXTRATESTFILES) $(EXTRAGOARGS)
	sudo env "PATH=$(PATH)" go test $(EXTRATESTFILES) $(EXTRAGOARGS) -args $(WITHSNAPSHOTS)
	./scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/fccd_orch_upf_log.out 2>$(CTRDLOGDIR)/fccd_orch_upf_log.err &
	sudo env "PATH=$(PATH)" go test $(EXTRATESTFILES) $(EXTRAGOARGS) -args $(WITHSNAPSHOTS) $(WITHUPF)
	./scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/fccd_orch_upf_lazy_log.out 2>$(CTRDLOGDIR)/fccd_orch_upf_lazy_log.err &
	sudo env "PATH=$(PATH)" go test $(EXTRATESTFILES) $(EXTRAGOARGS) -args $(WITHSNAPSHOTS) $(WITHUPF) $(WITHLAZY)
	./scripts/clean_fcctr.sh


test-man:
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/fccd_orch_noupf_log_man_travis.out 2>$(CTRDLOGDIR)/fccd_orch_noupf_log_man_travis.err &
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS_NORACE) -run TestParallelServe
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestServeThree
	#sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS_NORACE) -run TestParallelServe -args $(WITHSNAPSHOTS)
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestServeThree -args $(WITHSNAPSHOTS)
	./scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/fccd_orch_both_log_man_travis.out 2>$(CTRDLOGDIR)/fccd_orch_both_log_man_travis.err &
	#sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS_NORACE) -run TestParallelServe -args $(WITHSNAPSHOTS) $(WITHUPF)
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestServeThree -args $(WITHSNAPSHOTS) $(WITHUPF)
	./scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/fccd_orch_both_lazy_log_man_travis.out 2>$(CTRDLOGDIR)/fccd_orch_both_lazy_log_man_travis.err &
	#sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS_NORACE) -run TestParallelServe -args $(WITHSNAPSHOTS) $(WITHUPF) $(WITHLAZY)
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestServeThree -args $(WITHSNAPSHOTS) $(WITHUPF) $(WITHLAZY)
	./scripts/clean_fcctr.sh

test-skip:
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/fccd_orch_noupf_log_man_skip.out 2>$(CTRDLOGDIR)/fccd_orch_noupf_log_man_skip.err &
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS_NORACE) -run TestParallelServe -args $(WITHSNAPSHOTS)
	./scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/fccd_orch_both_log_man_skip.out 2>$(CTRDLOGDIR)/fccd_orch_both_log_man_skip.err &
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS_NORACE) -run TestParallelServe -args $(WITHSNAPSHOTS) $(WITHUPF)
	./scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/fccd_orch_both_lazy_log_man_skip.out 2>$(CTRDLOGDIR)/fccd_orch_both_lazy_log_man_skip.err &
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS_NORACE) -run TestParallelServe -args $(WITHSNAPSHOTS) $(WITHUPF) $(WITHLAZY)
	./scripts/clean_fcctr.sh

log-clean:
	sudo rm -rf $(CTRDLOGDIR)

$(SUBDIRS):
	$(MAKE) -C $@ test
	$(MAKE) -C $@ test-man

test-subdirs: $(SUBDIRS)

.PHONY: test-orch $(SUBDIRS) test-subdirs
