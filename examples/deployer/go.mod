module github.com/vhive-serverless/vhive/examples/deployer

go 1.16

replace github.com/vhive-serverless/vhive/examples/endpoint => ../endpoint

require (
	github.com/sirupsen/logrus v1.9.0
	github.com/vhive-serverless/vhive/examples/endpoint v0.0.0-00010101000000-000000000000
)
