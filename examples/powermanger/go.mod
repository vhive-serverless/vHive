module github.com/vhive-serverless/vhive/examples/deployer

go 1.19

replace github.com/vhive-serverless/vhive/examples/deployer => ../deployer

require github.com/shirou/gopsutil v3.21.11+incompatible

require (
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/stretchr/testify v1.8.4 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.3 // indirect
	golang.org/x/sys v0.13.0 // indirect
)
