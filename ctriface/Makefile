EXTRAGOARGS:=-v -race
EXTRATESTFILES:=iface_test.go iface.go dialer.go
MANUALTESTFILES:=manual_cleanup_test.go iface.go dialer.go

test:
	sudo env "PATH=$(PATH)" go test $(EXTRATESTFILES) $(EXTRAGOARGS)

test-man:
	sudo env "PATH=$(PATH)" go test $(MANUALTESTFILES) $(EXTRAGOARGS)
	../scripts/clean_fcctr.sh


.PHONY: test
