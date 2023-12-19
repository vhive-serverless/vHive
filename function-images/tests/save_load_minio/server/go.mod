module main

go 1.19

replace github.com/vhive-serverless/vhive/function-images/tests/save_load_minio/proto => ../proto_gen

require (
	github.com/containerd/containerd v1.6.26
	github.com/minio/minio-go/v7 v7.0.10
	github.com/sirupsen/logrus v1.9.3
	github.com/vhive-serverless/vhive/function-images/tests/save_load_minio/proto v0.0.0-00010101000000-000000000000
	google.golang.org/grpc v1.58.3
)

require (
	github.com/containerd/log v0.1.0 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/cpuid v1.3.1 // indirect
	github.com/minio/md5-simd v1.1.0 // indirect
	github.com/minio/sha256-simd v0.1.1 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/rs/xid v1.2.1 // indirect
	golang.org/x/crypto v0.14.0 // indirect
	golang.org/x/net v0.17.0 // indirect
	golang.org/x/sys v0.13.0 // indirect
	golang.org/x/text v0.13.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230711160842-782d3b101e98 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/ini.v1 v1.57.0 // indirect
)
