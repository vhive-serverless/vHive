EXTRAGOARGS:=-v -race -cover
CTRDLOGDIR:=/tmp/ctrd-logs
test:
	sudo mkdir -p -m777 -p $(CTRDLOGDIR)
	sudo containerd 1>$(CTRDLOGDIR)/ctrd.out 2>$(CTRDLOGDIR)/ctrd.err &
	sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/fccd.out 2>$(CTRDLOGDIR)/fccd.err &
	sudo ./../fccd-orchestrator 1>$(CTRDLOGDIR)/orch.out 2>$(CTRDLOGDIR)/orch.err &

	./../scripts/cluster/create_one_node_cluster.sh

	sleep 2m

	kubectl apply -f ../knative_workloads/helloworld.yaml
	kubectl apply -f ../knative_workloads/pyaes.yaml
	kubectl apply -f ../knative_workloads/rnn_serving.yaml

	sleep 1m

	go test ./ $(EXTRAGOARGS)
	./../scripts/clean_fcctr.sh

test-travis:
	sudo mkdir -p -m777 -p $(CTRDLOGDIR)
	sudo containerd 1>$(CTRDLOGDIR)/ctrd.out 2>$(CTRDLOGDIR)/ctrd.err &
	sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/fccd.out 2>$(CTRDLOGDIR)/fccd.err &
	sudo ./../fccd-orchestrator 1>$(CTRDLOGDIR)/orch.out 2>$(CTRDLOGDIR)/orch.err &

	./../scripts/cluster/create_one_node_cluster.sh

	sleep 2m

	kubectl apply -f ../knative_workloads/helloworld.yaml

	sleep 2m

	go test $(EXTRAGOARGS) -run TestSingleInvoke
	./../scripts/clean_fcctr.sh

test-man:
	echo "Nothing to test manually"

.PHONY: test test-man
