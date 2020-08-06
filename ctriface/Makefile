EXTRAGOARGS:=-v -race -cover
EXTRATESTFILES:=iface_test.go iface.go dialer.go orch_options.go
MANUALTESTFILES:=manual_cleanup_test.go iface.go dialer.go orch_options.go
BENCHFILES:=bench_test.go iface.go dialer.go orch_options.go
WITHUPF:=-upf
GOBENCH:=-v -timeout 1500s
CTRDLOGDIR:=/tmp/ctrd-logs

test:
	sudo mkdir -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/ctriface_log.out 2>$(CTRDLOGDIR)/ctriface_log.err &
	sudo env "PATH=$(PATH)" go test $(EXTRATESTFILES) $(EXTRAGOARGS)
	sudo env "PATH=$(PATH)" go test $(EXTRATESTFILES) $(EXTRAGOARGS) -args $(WITHUPF)
	./../scripts/clean_fcctr.sh

test-man:
	sudo mkdir -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/ctriface_log_noupf_man.out 2>$(CTRDLOGDIR)/ctriface_log_noupf_man.err &
	sudo env "PATH=$(PATH)" go test $(MANUALTESTFILES) $(EXTRAGOARGS)
	./../scripts/clean_fcctr.sh
	sudo mkdir -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/ctriface_log_upf_man.out 2>$(CTRDLOGDIR)/ctriface_log_upf_man.err &
	sudo env "PATH=$(PATH)" go test $(MANUALTESTFILES) $(EXTRAGOARGS) -args $(WITHUPF)
	./../scripts/clean_fcctr.sh

bench:
	sudo env "PATH=$(PATH)" go test $(BENCHFILES) $(GOBENCH)
	./../scripts/clean_fcctr.sh
.PHONY: test test-man test-man-upf bench
