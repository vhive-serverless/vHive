EXTRAGOARGS:=-v -race -cover
EXTRATESTFILES:=iface_test.go iface.go dialer.go orch_options.go
MANUALTESTFILES:=manual_cleanup_test.go iface.go dialer.go orch_options.go
BENCHFILES:=bench_test.go iface.go dialer.go orch_options.go
GOBENCH:=-v -timeout 1500s

test:
	sudo env "PATH=$(PATH)" go test $(EXTRATESTFILES) $(EXTRAGOARGS)

test-man:
	sudo env "PATH=$(PATH)" go test $(MANUALTESTFILES) $(EXTRAGOARGS)
	./../scripts/clean_fcctr.sh

bench:
	sudo env "PATH=$(PATH)" go test $(BENCHFILES) $(GOBENCH)
	./../scripts/clean_fcctr.sh
.PHONY: test test-man bench
