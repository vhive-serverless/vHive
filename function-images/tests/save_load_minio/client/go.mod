module main

go 1.23.0

replace github.com/vhive-serverless/vhive/function-images/tests/save_load_minio/proto => ../proto_gen

require (
	github.com/containerd/containerd v1.7.29
	github.com/sirupsen/logrus v1.9.3
	github.com/vhive-serverless/vhive/function-images/tests/save_load_minio/proto v0.0.0-00010101000000-000000000000
	google.golang.org/grpc v1.59.0
)

require (
	github.com/containerd/log v0.1.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	golang.org/x/net v0.42.0 // indirect
	golang.org/x/sys v0.34.0 // indirect
	golang.org/x/text v0.27.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240401170217-c3f982113cda // indirect
	google.golang.org/protobuf v1.35.2 // indirect
)
