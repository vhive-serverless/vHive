# MIT License
#
# Copyright (c) 2020 Plamen Petrov and EASE lab
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#
# THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
# IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
# FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
# AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
# LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
# OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
# SOFTWARE.

EXTRAGOARGS:=-v -race -cover
test:

	./../scripts/cloudlab/start_onenode_vhive_cluster.sh

	sleep 2m
	KUBECONFIG=/etc/kubernetes/admin.conf ./../scripts/setup_zipkin.sh
	sleep 5s

	go run ./../examples/registry/populate_registry.go -imageFile ./../examples/registry/images.txt
	sleep 1s

	KUBECONFIG=/etc/kubernetes/admin.conf kn service apply helloworld -f ./../configs/knative_workloads/helloworld.yaml
	KUBECONFIG=/etc/kubernetes/admin.conf kn service apply helloworldlocal -f ./../configs/knative_workloads/helloworld_local.yaml
	KUBECONFIG=/etc/kubernetes/admin.conf kn service apply helloworldserial -f ./../configs/knative_workloads/helloworldSerial.yaml
	KUBECONFIG=/etc/kubernetes/admin.conf kn service apply pyaes -f ./../configs/knative_workloads/pyaes.yaml
	sleep 1m

	go test ./... $(EXTRAGOARGS)

test-man:
	echo "Nothing to test manually"

.PHONY: test test-man
