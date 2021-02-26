module main

go 1.15

replace github.com/ease-lab/vhive/function-images/tests/save_load_minio/proto => ../proto_gen

require (
	github.com/containerd/containerd v1.4.3
	github.com/ease-lab/vhive/function-images/tests/save_load_minio/proto v0.0.0-00010101000000-000000000000
	github.com/minio/minio-go/v7 v7.0.10
	github.com/sirupsen/logrus v1.8.0
	google.golang.org/grpc v1.36.0
)
