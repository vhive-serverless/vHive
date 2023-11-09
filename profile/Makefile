EXTRAGOARGS:=-v -race -cover

test:
	sudo env "PATH=$(PATH)" go test ./plotter*.go $(EXTRAGOARGS)

test-man:
	echo "Nothing to test manually"

.PHONY: test test-man