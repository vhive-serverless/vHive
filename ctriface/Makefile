EXTRAGOARGS:=-v -race -cover
EXTRATESTFILES:=iface_test.go iface.go dialer.go
MANUALTESTFILES:=manual_cleanup_test.go iface.go dialer.go

test:
	sudo env "PATH=$(PATH)" go test $(EXTRATESTFILES) $(EXTRAGOARGS)

test-man:
	sudo env "PATH=$(PATH)" go test $(MANUALTESTFILES) $(EXTRAGOARGS)


.PHONY: test test-man
