EXTRAGOARGS:=-v -race -cover
EXTRATESTFILES:=iface_test.go iface.go dialer.go orch_options.go
MANUALTESTFILES:=manual_cleanup_test.go iface.go dialer.go orch_options.go

test:
	sudo env "PATH=$(PATH)" go test $(EXTRATESTFILES) $(EXTRAGOARGS)

test-man:
	sudo env "PATH=$(PATH)" go test $(MANUALTESTFILES) $(EXTRAGOARGS)


.PHONY: test test-man
