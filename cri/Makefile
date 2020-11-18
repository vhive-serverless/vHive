EXTRAGOARGS:=-v -race -cover

test:
	CTRDLOG=$HOME/ctrdlog
	mkdir -p $CTRDLOG

	cd ..
	go build ./...
	containerd &
	sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/log.out 2>$(CTRDLOGDIR)/log.err &
	./fccd-orchestrator &

	./scripts/cri/create_one_node_cluster.sh

	sleep 2m

	kubectl apply -f ../../knative_workloads/helloworld.yaml
	kubectl apply -f ../../knative_workloads/pyaes.yaml
	kubectl apply -f ../../knative_workloads/rnn_serving.yaml

	sleep 1m

	cd cri
	go test ./ $(EXTRAGOARGS)
	./../scripts/clean_fcctr.sh

test-man:
	echo "Nothing to test manually"

.PHONY: test test-man
