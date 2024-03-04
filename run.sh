./scripts/clean_fcctr.sh
./scripts/cloudlab/setup_node.sh
go build -race -v -a ./...
make debug > output.log 2>&1
code output.log
