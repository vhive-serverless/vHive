module main

go 1.16

replace github.com/ease-lab/vhive/function-images/tests/save_load_minio/proto => ../proto_gen

require (
	github.com/containerd/containerd v1.5.13
	github.com/ease-lab/vhive/function-images/tests/save_load_minio/proto v0.0.0-00010101000000-000000000000
	github.com/sirupsen/logrus v1.8.1
	google.golang.org/grpc v1.36.0
)
