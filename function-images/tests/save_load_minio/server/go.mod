module main

go 1.16

replace github.com/ease-lab/vhive/function-images/tests/save_load_minio/proto => ../proto_gen

require (
	github.com/containerd/containerd v1.4.8
	github.com/ease-lab/vhive/function-images/tests/save_load_minio/proto v0.0.0-00010101000000-000000000000
	github.com/google/uuid v1.2.0 // indirect
	github.com/minio/minio-go/v7 v7.0.10
	github.com/pkg/errors v0.9.1 // indirect
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.6.1 // indirect
	golang.org/x/crypto v0.0.0-20210322153248-0c34fe9e7dc2 // indirect
	golang.org/x/sys v0.0.0-20210426230700-d19ff857e887 // indirect
	golang.org/x/text v0.3.4 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/genproto v0.0.0-20201110150050-8816d57aaa9a // indirect
	google.golang.org/grpc v1.36.0
	google.golang.org/protobuf v1.26.0 // indirect
	gotest.tools/v3 v3.0.3 // indirect
)
