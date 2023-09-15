package template

// adopted from vSwarm[https://github.com/vhive-serverless/vSwarm/blob/main/benchmarks/aes/yamls/knative/kn-aes-python.yaml]
const (
	benchmarkTemplate = `echo 'apiVersion: serving.knative.dev/v1
kind: Service
metadata:
  name: helloworld-python-%s
  namespace: default
spec:
  template:
    spec:
      nodeSelector:
        apps.openyurt.io/nodepool: %s
      containers:
        - image: docker.io/vhiveease/hello-%s:latest
          ports:
            - name: h2c
              containerPort: 50000' > %s`
)

func GetBenchmarkTemplate() string {
	return benchmarkTemplate
}
