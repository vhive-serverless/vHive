sudo modprobe br_netfilter && sudo modprobe overlay && sudo sysctl -w net.ipv4.ip_forward=1 && sudo sysctl -w net.ipv4.conf.all.forwarding=1
sudo sysctl -w net.bridge.bridge-nf-call-iptables=1 && sudo sysctl -w net.bridge.bridge-nf-call-ip6tables=1
sudo mkdir -p /etc/firecracker-containerd && sudo mkdir -p /var/lib/firecracker-containerd/runtime && sudo mkdir -p /etc/containerd/

git lfs fetch
git lfs checkout

sudo cp bin/{firecracker,jailer,containerd-shim-aws-firecracker,firecracker-containerd,firecracker-ctr} /usr/local/bin
sudo cp bin/default-rootfs.img /var/lib/firecracker-containerd/runtime
sudo cp bin/vmlinux-5.10.186 /var/lib/firecracker-containerd/runtime/hello-vmlinux.bin
sudo cp configs/firecracker-containerd/config.toml /etc/firecracker-containerd/
sudo cp configs/firecracker-containerd/firecracker-runtime.json /etc/containerd/

./scripts/stargz/setup_demux_snapshotter.sh
./scripts/stargz/setup_stargz.sh

tmux new -s firecracker -d
tmux send -t firecracker "sudo PATH=$PATH /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 2>&1 | tee ~/firecracker_log.txt" ENTER
tmux new -s http-address-resolver -d
tmux send -t http-address-resolver "sudo PATH=$PATH /usr/local/bin/http-address-resolver" ENTER
tmux new -s demux-snapshotter -d
tmux send -t demux-snapshotter 'while true; do sudo /usr/local/bin/demux-snapshotter; done' ENTER

pushd ~; git clone https://github.com/vhive-serverless/vswarm; source /etc/profile; cd vswarm/tools/relay; make relay; popd
