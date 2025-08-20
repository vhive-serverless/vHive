# MIT License
#
# Copyright (c) 2023 Georgiy Lebedev, Dmitrii Ustiugov, Plamen Petrov and vHive team
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.

SUBDIRS:=ctriface taps misc profile
EXTRAGOARGS:=-v -race -cover
EXTRAGOARGS_NORACE:=-v
EXTRATESTFILES:=vhive_test.go stats.go vhive.go functions.go
# User-level page faults are temporarily disabled (gh-807)
# WITHUPF:=-upfTest
# WITHLAZY:=-lazyTest
WITHUPF:=
WITHLAZY:=
WITHSNAPSHOTS:=-snapshotsTest
CTRDLOGDIR:=/tmp/ctrd-logs
FIRECRACKER_BIN?=/usr/local/bin/firecracker-containerd
FIRECRACKER_CONFIG_PATH?=/etc/firecracker-containerd/config.toml

vhive: proto
	go install github.com/vhive-serverless/vhive

protobuf:
	protoc -I proto/ proto/orchestrator.proto --go_out=plugins=grpc:proto

clean:
	rm proto/orchestrator.pb.go

test-all: test-subdirs test-orch

test-orch: test test-man

test:
	./scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR)

	sudo env \
		"PATH=$(PATH)" \
		"FIRECRACKER_BIN=$(FIRECRACKER_BIN)" \
		"FIRECRACKER_CONFIG_PATH=$(FIRECRACKER_CONFIG_PATH)" \
		"LOG_OUT=$(CTRDLOGDIR)/fccd_orch_noupf_log.out" \
		"LOG_ERR=$(CTRDLOGDIR)/fccd_orch_noupf_log.err" \
		go test $(EXTRATESTFILES) -short $(EXTRAGOARGS)

	sudo env \
		"PATH=$(PATH)" \
		"FIRECRACKER_BIN=$(FIRECRACKER_BIN)" \
		"FIRECRACKER_CONFIG_PATH=$(FIRECRACKER_CONFIG_PATH)" \
		"LOG_OUT=$(CTRDLOGDIR)/fccd_orch_noupf_log.out" \
		"LOG_ERR=$(CTRDLOGDIR)/fccd_orch_noupf_log.err" \
		go test $(EXTRATESTFILES) -short $(EXTRAGOARGS) -args $(WITHSNAPSHOTS)

	sudo env \
		"PATH=$(PATH)" \
		"FIRECRACKER_BIN=$(FIRECRACKER_BIN)" \
		"FIRECRACKER_CONFIG_PATH=$(FIRECRACKER_CONFIG_PATH)" \
		"LOG_OUT=$(CTRDLOGDIR)/fccd_orch_upf_log.out" \
		"LOG_ERR=$(CTRDLOGDIR)/fccd_orch_upf_log.err" \
		go test $(EXTRATESTFILES) -short $(EXTRAGOARGS) -args $(WITHSNAPSHOTS) $(WITHUPF)

	sudo env \
		"PATH=$(PATH)" \
		"FIRECRACKER_BIN=/usr/local/bin/firecracker-containerd" \
		"FIRECRACKER_CONFIG_PATH=/etc/firecracker-containerd/config.toml" \
		"LOG_OUT=$(CTRDLOGDIR)/fccd_orch_upf_lazy_log.out" \
		"LOG_ERR=$(CTRDLOGDIR)/fccd_orch_upf_lazy_log.err" \
		go test $(EXTRATESTFILES) -short $(EXTRAGOARGS) -args $(WITHSNAPSHOTS) $(WITHUPF) $(WITHLAZY)

	sudo env \
		"PATH=$(PATH)" \
		"FIRECRACKER_BIN=$(FIRECRACKER_BIN)" \
		"FIRECRACKER_CONFIG_PATH=$(FIRECRACKER_CONFIG_PATH)" \
		"LOG_OUT=$(CTRDLOGDIR)/fccd_orch_noupf_log_bench.out" \
		"LOG_ERR=$(CTRDLOGDIR)/fccd_orch_noupf_log_bench.err" \
		go test -short $(EXTRAGOARGS) -run TestProfileSingleConfiguration \
			-args -test -loadStep 100 && sudo rm -rf bench_results

	sudo env \
		"PATH=$(PATH)" \
		"FIRECRACKER_BIN=$(FIRECRACKER_BIN)" \
		"FIRECRACKER_CONFIG_PATH=$(FIRECRACKER_CONFIG_PATH)" \
		"LOG_OUT=$(CTRDLOGDIR)/fccd_orch_noupf_log_bench.out" \
		"LOG_ERR=$(CTRDLOGDIR)/fccd_orch_noupf_log_bench.err" \
		go test -short $(EXTRAGOARGS) -run TestProfileIncrementConfiguration \
			-args -test -vmIncrStep 4 -maxVMNum 4 -loadStep 100 && sudo rm -rf bench_results

	sudo env \
		"PATH=$(PATH)" \
		"FIRECRACKER_BIN=$(FIRECRACKER_BIN)" \
		"FIRECRACKER_CONFIG_PATH=$(FIRECRACKER_CONFIG_PATH)" \
		"LOG_OUT=$(CTRDLOGDIR)/fccd_orch_noupf_log_bench.out" \
		"LOG_ERR=$(CTRDLOGDIR)/fccd_orch_noupf_log_bench.err" \
		go test -short $(EXTRAGOARGS) -run TestBindSocket

test-man:
	./scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR)

	sudo env \
		"PATH=$(PATH)" \
		"FIRECRACKER_BIN=$(FIRECRACKER_BIN)" \
		"FIRECRACKER_CONFIG_PATH=$(FIRECRACKER_CONFIG_PATH)" \
		"LOG_OUT=$(CTRDLOGDIR)/fccd_orch_noupf_log_man_travis.out" \
		"LOG_ERR=$(CTRDLOGDIR)/fccd_orch_noupf_log_man_travis.err" \
		go test $(EXTRAGOARGS_NORACE) -run TestParallelServe
	
	sudo env \
		"PATH=$(PATH)" \
		"FIRECRACKER_BIN=$(FIRECRACKER_BIN)" \
		"FIRECRACKER_CONFIG_PATH=$(FIRECRACKER_CONFIG_PATH)" \
		"LOG_OUT=$(CTRDLOGDIR)/fccd_orch_noupf_log_man_travis.out" \
		"LOG_ERR=$(CTRDLOGDIR)/fccd_orch_noupf_log_man_travis.err" \
		go test $(EXTRAGOARGS) -run TestServeThree

	sudo env \
		"PATH=$(PATH)" \
		"FIRECRACKER_BIN=$(FIRECRACKER_BIN)" \
		"FIRECRACKER_CONFIG_PATH=$(FIRECRACKER_CONFIG_PATH)" \
		"LOG_OUT=$(CTRDLOGDIR)/fccd_orch_noupf_log_man_travis.out" \
		"LOG_ERR=$(CTRDLOGDIR)/fccd_orch_noupf_log_man_travis.err" \
		go test $(EXTRAGOARGS) -run TestServeThree -args $(WITHSNAPSHOTS)

	sudo env \
		"PATH=$(PATH)" \
		"FIRECRACKER_BIN=$(FIRECRACKER_BIN)" \
		"FIRECRACKER_CONFIG_PATH=$(FIRECRACKER_CONFIG_PATH)" \
		"LOG_OUT=$(CTRDLOGDIR)/fccd_orch_both_log_man_travis.out" \
		"LOG_ERR=$(CTRDLOGDIR)/fccd_orch_both_log_man_travis.err" \
		go test $(EXTRAGOARGS) -run TestServeThree -args $(WITHSNAPSHOTS) $(WITHUPF)

	sudo env \
		"PATH=$(PATH)" \
		"FIRECRACKER_BIN=$(FIRECRACKER_BIN)" \
		"FIRECRACKER_CONFIG_PATH=$(FIRECRACKER_CONFIG_PATH)" \
		"LOG_OUT=$(CTRDLOGDIR)/fccd_orch_both_lazy_log_man_travis.out" \
		"LOG_ERR=$(CTRDLOGDIR)/fccd_orch_both_lazy_log_man_travis.err" \
		go test $(EXTRAGOARGS) -run TestServeThree -args $(WITHSNAPSHOTS) $(WITHUPF) $(WITHLAZY)

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

bench:
	./scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR)
	
	sudo env \
		"PATH=$(PATH)" \
		"FIRECRACKER_BIN=$(FIRECRACKER_BIN)" \
		"FIRECRACKER_CONFIG_PATH=$(FIRECRACKER_CONFIG_PATH)" \
		"LOG_OUT=$(CTRDLOGDIR)/fccd_orch_noupf_log_bench.out" \
		"LOG_ERR=$(CTRDLOGDIR)/fccd_orch_noupf_log_bench.err" \
		go test $(EXTRAGOARGS) -run TestBenchServe -args -iter 1 $(WITHSNAPSHOTS) -benchDirTest configBase -metricsTest -funcName helloworld && sudo rm -rf configBase
	
	sudo env \
		"PATH=$(PATH)" \
		"FIRECRACKER_BIN=$(FIRECRACKER_BIN)" \
		"FIRECRACKER_CONFIG_PATH=$(FIRECRACKER_CONFIG_PATH)" \
		"LOG_OUT=$(CTRDLOGDIR)/fccd_orch_noupf_log_bench.out" \
		"LOG_ERR=$(CTRDLOGDIR)/fccd_orch_noupf_log_bench.err" \
		go test $(EXTRAGOARGS) -run TestBenchServe -args -iter 1 $(WITHSNAPSHOTS) $(WITHUPF) -benchDirTest configREAP -metricsTest -funcName helloworld && sudo rm -rf configREAP

	sudo env \
		"PATH=$(PATH)" \
		"FIRECRACKER_BIN=$(FIRECRACKER_BIN)" \
		"FIRECRACKER_CONFIG_PATH=$(FIRECRACKER_CONFIG_PATH)" \
		"LOG_OUT=$(CTRDLOGDIR)/fccd_orch_noupf_log_bench.out" \
		"LOG_ERR=$(CTRDLOGDIR)/fccd_orch_noupf_log_bench.err" \
		go test $(EXTRAGOARGS) -run TestBenchServe -args -iter 1 $(WITHSNAPSHOTS) $(WITHLAZY) -benchDirTest configLazy -metricsTest -funcName helloworld && sudo rm -rf configLazy

	sudo env \
		"PATH=$(PATH)" \
		"FIRECRACKER_BIN=$(FIRECRACKER_BIN)" \
		"FIRECRACKER_CONFIG_PATH=$(FIRECRACKER_CONFIG_PATH)" \
		"LOG_OUT=$(CTRDLOGDIR)/fccd_orch_noupf_log_bench.out" \
		"LOG_ERR=$(CTRDLOGDIR)/fccd_orch_noupf_log_bench.err" \
		go test $(EXTRAGOARGS) -run TestBenchParallelServe -args $(WITHSNAPSHOTS) -benchDirTest configBase -metricsTest -funcName helloworld && sudo rm -rf configBase
	
	sudo env \
		"PATH=$(PATH)" \
		"FIRECRACKER_BIN=$(FIRECRACKER_BIN)" \
		"FIRECRACKER_CONFIG_PATH=$(FIRECRACKER_CONFIG_PATH)" \
		"LOG_OUT=$(CTRDLOGDIR)/fccd_orch_noupf_log_bench.out" \
		"LOG_ERR=$(CTRDLOGDIR)/fccd_orch_noupf_log_bench.err" \
		go test $(EXTRAGOARGS) -run TestBenchParallelServe -args $(WITHSNAPSHOTS) $(WITHUPF) -benchDirTest configREAP -metricsTest -funcName helloworld && sudo rm -rf configREAP
	
	sudo env \
		"PATH=$(PATH)" \
		"FIRECRACKER_BIN=$(FIRECRACKER_BIN)" \
		"FIRECRACKER_CONFIG_PATH=$(FIRECRACKER_CONFIG_PATH)" \
		"LOG_OUT=$(CTRDLOGDIR)/fccd_orch_noupf_log_bench.out" \
		"LOG_ERR=$(CTRDLOGDIR)/fccd_orch_noupf_log_bench.err" \
		go test $(EXTRAGOARGS) -run TestBenchParallelServe -args $(WITHSNAPSHOTS) $(WITHLAZY) -benchDirTest configLazy -metricsTest -funcName helloworld && sudo rm -rf configLazy

test-man-bench:
	$(MAKE) test-man
	$(MAKE) bench

nightly-test:
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/fccd_orch_noupf_log.out 2>$(CTRDLOGDIR)/fccd_orch_noupf_log.err &
	sudo env "PATH=$(PATH)" go test $(EXTRATESTFILES) -run TestAllFunctions $(EXTRAGOARGS)
	sudo env "PATH=$(PATH)" go test $(EXTRATESTFILES) -run TestAllFunctions $(EXTRAGOARGS) -args $(WITHSNAPSHOTS)
	./scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/fccd_orch_upf_log.out 2>$(CTRDLOGDIR)/fccd_orch_upf_log.err &
	sudo env "PATH=$(PATH)" go test $(EXTRATESTFILES) -run TestAllFunctions $(EXTRAGOARGS) -args $(WITHSNAPSHOTS) $(WITHUPF)
	./scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/fccd_orch_upf_lazy_log.out 2>$(CTRDLOGDIR)/fccd_orch_upf_lazy_log.err &
	sudo env "PATH=$(PATH)" go test $(EXTRATESTFILES) -run TestAllFunctions $(EXTRAGOARGS) -args $(WITHSNAPSHOTS) $(WITHUPF) $(WITHLAZY)
	./scripts/clean_fcctr.sh

test-skip-all:
	$(MAKE) test-skip
	$(MAKE) -C ctriface test-skip

log-clean:
	sudo rm -rf $(CTRDLOGDIR)

$(SUBDIRS):
	$(MAKE) -C $@ test
	$(MAKE) -C $@ test-man

test-subdirs: $(SUBDIRS)

test-cri:
	$(MAKE) -C cri test-cri-firecracker

test-cri-gvisor:
	$(MAKE) -C cri test-cri-gvisor

test-cri-travis: # Testing in travis is deprecated
	$(MAKE) -C cri test-travis

.PHONY: test-orch $(SUBDIRS) test-subdirs
