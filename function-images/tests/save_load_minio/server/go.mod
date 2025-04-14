module main

go 1.19

replace github.com/vhive-serverless/vhive/function-images/tests/save_load_minio/proto => ../proto_gen

require (
	github.com/containerd/containerd v1.5.2
	github.com/minio/minio-go/v7 v7.0.10
	github.com/sirupsen/logrus v1.8.1
	github.com/vhive-serverless/vhive/function-images/tests/save_load_minio/proto v0.0.0-00010101000000-000000000000
	google.golang.org/grpc v1.54.0
)

require (
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
	golang.org/x/crypto v0.35.0 // indirect
	golang.org/x/net v0.21.0 // indirect
	golang.org/x/sys v0.30.0 // indirect
	golang.org/x/text v0.22.0 // indirect
	google.golang.org/genproto v0.0.0-20230110181048-76db0878b65f // indirect
	google.golang.org/protobuf v1.30.0 // indirect
	gopkg.in/ini.v1 v1.57.0 // indirect
)
