EXTRAGOARGS:=-v -race -cover
EXTRATESTFILES:=iface_test.go iface.go dialer.go orch_options.go
MANUALTESTFILES:=manual_cleanup_test.go iface.go dialer.go orch_options.go
BENCHFILES:=bench_test.go iface.go dialer.go orch_options.go
WITHUPF:=-upf
WITHLAZY:=-lazy
GOBENCH:=-v -timeout 1500s
CTRDLOGDIR:=/tmp/ctrd-logs

test:
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/ctriface_log.out 2>$(CTRDLOGDIR)/ctriface_log.err &
	sudo env "PATH=$(PATH)" go test $(EXTRATESTFILES) $(EXTRAGOARGS)
	sudo env "PATH=$(PATH)" go test $(EXTRATESTFILES) $(EXTRAGOARGS) -args $(WITHUPF)
	./../scripts/clean_fcctr.sh

test-man:
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/ctriface_log_noupf_man_travis.out 2>$(CTRDLOGDIR)/ctriface_log_noupf_man_travis.err &
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestSnapLoad
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestSnapLoadMultiple
	./../scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/ctriface_log_upf_man_travis.out 2>$(CTRDLOGDIR)/ctriface_log_upf_man_travis.err &
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestSnapLoad -args $(WITHUPF)
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestSnapLoadMultiple -args $(WITHUPF)
	./../scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/ctriface_log_lazy_man_travis.out 2>$(CTRDLOGDIR)/ctriface_log_lazy_man_travis.err &
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestSnapLoad -args $(WITHUPF) $(WITHLAZY)
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestSnapLoadMultiple -args $(WITHUPF) $(WITHLAZY)
	./../scripts/clean_fcctr.sh

test-skip:
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/ctriface_log_noupf_man_skip.out 2>$(CTRDLOGDIR)/ctriface_log_noupf_man_skip.err &
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestParallelSnapLoad
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestParallelPhasedSnapLoad
	./../scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/ctriface_log_upf_man_skip.out 2>$(CTRDLOGDIR)/ctriface_log_upf_man_skip.err &
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestParallelSnapLoad -args $(WITHUPF)
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestParallelPhasedSnapLoad -args $(WITHUPF)
	./../scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/ctriface_log_lazy_man_skip.out 2>$(CTRDLOGDIR)/ctriface_log_lazy_man_skip.err &
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestParallelSnapLoad -args $(WITHUPF) $(WITHLAZY)
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestParallelPhasedSnapLoad -args $(WITHUPF) $(WITHLAZY)
	./../scripts/clean_fcctr.sh


bench:
	sudo env "PATH=$(PATH)" go test $(BENCHFILES) $(GOBENCH)
	./../scripts/clean_fcctr.sh
.PHONY: test test-man test-man-upf bench
