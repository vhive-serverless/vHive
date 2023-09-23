package template

const (
	YurtAppSetTemplate = `apiVersion: apps.openyurt.io/v1beta1
kind: YurtAppSet
metadata:
labels:
    controller-tools.k8s.io: 1.0
    name: yas-test
spec:
selector:
    matchLabels:
    app: yas-test
workloadTemplate:
deploymentTemplate:
    metadata:
    labels:
        app: yas-test
    spec:
    template:
        metadata:
        labels:
            app: yas-test
        spec:
        containers: # can be changed to your own images
            - name: srcnn
            image: lrq619/srcnn
            ports:
                - containerPort: 8000 # the port docker exposes
topology:
    pools:
    - name: beijing # cloud nodepool name`
)

func GetYurtAppSetTemplate() string {
	return YurtAppSetTemplate
}
