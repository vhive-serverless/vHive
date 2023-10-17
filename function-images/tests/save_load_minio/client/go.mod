module main

go 1.19

replace github.com/vhive-serverless/vhive/function-images/tests/save_load_minio/proto => ../proto_gen

require (
	github.com/containerd/containerd v1.4.3
	github.com/sirupsen/logrus v1.8.0
	github.com/vhive-serverless/vhive/function-images/tests/save_load_minio/proto v0.0.0-00010101000000-000000000000
	google.golang.org/grpc v1.54.0
)

require (
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/magefile/mage v1.10.0 // indirect
	golang.org/x/net v0.17.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	google.golang.org/genproto v0.0.0-20230110181048-76db0878b65f // indirect
	google.golang.org/protobuf v1.30.0 // indirect
	gotest.tools/v3 v3.4.0 // indirect
)
