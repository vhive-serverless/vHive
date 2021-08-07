set -x

mkdir -p "$@"

for tests in "$@/*/" ; do
    for test in $tests; do
        ../../function-images/tests/kn_deploy.sh "./$test*.yml"
        sleep 2s
        go run invoke.go -endpoint driver.default.192.168.1.240.sslip.io -port 80 -sampleSize 10 -latf ${test}lat.csv
        sleep 1s
        go run invoke.go -endpoint driver.default.192.168.1.240.sslip.io -port 80 -sampleSize 10 -latf ${test}lat.csv
        kn service delete --all --wait
    done
done