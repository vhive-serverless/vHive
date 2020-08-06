EXTRAGOARGS:=-v -race -cover
EXTRATESTFILES:=iface_test.go iface.go dialer.go orch_options.go
MANUALTESTFILES:=manual_cleanup_test.go iface.go dialer.go orch_options.go
BENCHFILES:=bench_test.go iface.go dialer.go orch_options.go
WITHUPF:=-upf true
GOBENCH:=-v -timeout 1500s

test:
	sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>log.out 2>log.err &
	sudo env "PATH=$(PATH)" go test $(EXTRATESTFILES) $(EXTRAGOARGS)
	sudo env "PATH=$(PATH)" go test $(EXTRATESTFILES) $(EXTRAGOARGS) -args $(WITHUPF)
	./../scripts/clean_fcctr.sh

test-man:
	sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>log_man.out 2>log_man.err &
	sudo env "PATH=$(PATH)" go test $(MANUALTESTFILES) $(EXTRAGOARGS)
	./../scripts/clean_fcctr.sh
	sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>>log_man.out 2>>log_man.err &
	sudo env "PATH=$(PATH)" go test $(MANUALTESTFILES) $(EXTRAGOARGS) -args $(WITHUPF)
	./../scripts/clean_fcctr.sh

bench:
	sudo env "PATH=$(PATH)" go test $(BENCHFILES) $(GOBENCH)
	./../scripts/clean_fcctr.sh
.PHONY: test test-man test-man-upf bench
