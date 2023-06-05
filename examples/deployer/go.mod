module github.com/vhive-serverless/vhive/examples/deployer

go 1.19

replace github.com/vhive-serverless/vhive/examples/endpoint => ../endpoint

require (
	github.com/sirupsen/logrus v1.9.3
	github.com/vhive-serverless/vhive/examples/endpoint v0.0.0-00010101000000-000000000000
)

require golang.org/x/sys v0.0.0-20220715151400-c0bba94af5f8 // indirect
