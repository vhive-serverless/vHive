package template

const (
	edgeNpTemplate = `apiVersion: apps.openyurt.io/v1beta1
kind: NodePool
metadata:
  name: worker
spec:
  type: Edge
  annotations:
    apps.openyurt.io/example: test-hangzhou
  labels:
    apps.openyurt.io/example: test-hangzhou
  taints:
  - key: apps.openyurt.io/example
    value: test-hangzhou
    effect: NoSchedule`
)

func GetEdgeNpTemplateConfig() string {
	return edgeNpTemplate
}
