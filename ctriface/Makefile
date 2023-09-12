# MIT License
#
# Copyright (c) 2023 Georgiy Lebedev, Dmitrii Ustiugov, Plamen Petrov and vHive team
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
EXTRATESTFILES:=iface_test.go iface.go orch_options.go orch.go
BENCHFILES:=bench_test.go iface.go orch_options.go orch.go
# User-level page faults are temporarily disabled (gh-807)
# WITHUPF:=-upf
# WITHLAZY:=-lazy
WITHUPF:=
WITHLAZY:=
GOBENCH:=-v -timeout 1500s
CTRDLOGDIR:=/tmp/ctrd-logs

test:
	./../scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/ctriface_log.out 2>$(CTRDLOGDIR)/ctriface_log.err &
	sudo env "PATH=$(PATH)" go test $(EXTRATESTFILES) $(EXTRAGOARGS)
	./../scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/ctriface_log.out 2>$(CTRDLOGDIR)/ctriface_log.err &
	sudo env "PATH=$(PATH)" go test $(EXTRATESTFILES) $(EXTRAGOARGS) -args $(WITHUPF)
	./../scripts/clean_fcctr.sh

test-man:
	./../scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/ctriface_log_noupf_man_travis.out 2>$(CTRDLOGDIR)/ctriface_log_noupf_man_travis.err &
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestSnapLoad
	./../scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/ctriface_log_upf_man_travis.out 2>$(CTRDLOGDIR)/ctriface_log_upf_man_travis.err &
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestSnapLoad -args $(WITHUPF)
	./../scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/ctriface_log_lazy_man_travis.out 2>$(CTRDLOGDIR)/ctriface_log_lazy_man_travis.err &
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestSnapLoad -args $(WITHUPF) $(WITHLAZY)
	./../scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/ctriface_log_remote_snap_create_man_travis.out 2>$(CTRDLOGDIR)/ctriface_log_remote_snap_create_man_travis.err &
	# Creates a remote snapshot.
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestRemoteSnapCreate
	# Cleans up the node.
	./../scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/ctriface_log_remote_snap_load_man_travis.out 2>$(CTRDLOGDIR)/ctriface_log_remote_snap_load_man_travis.err &
	# Loads the remote snapshot.
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestRemoteSnapLoad
	./../scripts/clean_fcctr.sh

test-skip:
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/ctriface_log_noupf_man_skip.out 2>$(CTRDLOGDIR)/ctriface_log_noupf_man_skip.err &
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestParallelSnapLoad
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestParallelPhasedSnapLoad
	./../scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/ctriface_log_upf_man_skip.out 2>$(CTRDLOGDIR)/ctriface_log_upf_man_skip.err &
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestParallelSnapLoad -args $(WITHUPF)
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestParallelPhasedSnapLoad -args $(WITHUPF)
	./../scripts/clean_fcctr.sh
	sudo mkdir -m777 -p $(CTRDLOGDIR) && sudo env "PATH=$(PATH)" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>$(CTRDLOGDIR)/ctriface_log_lazy_man_skip.out 2>$(CTRDLOGDIR)/ctriface_log_lazy_man_skip.err &
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestParallelSnapLoad -args $(WITHUPF) $(WITHLAZY)
	sudo env "PATH=$(PATH)" go test $(EXTRAGOARGS) -run TestParallelPhasedSnapLoad -args $(WITHUPF) $(WITHLAZY)
	./../scripts/clean_fcctr.sh


bench:
	sudo env "PATH=$(PATH)" go test $(BENCHFILES) $(GOBENCH)
	./../scripts/clean_fcctr.sh
.PHONY: test test-man test-man-upf bench
