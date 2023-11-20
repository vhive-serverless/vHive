package template

const (
	edgeTemplate = `echo 'apiVersion: apps.openyurt.io/v1beta1
kind: NodePool
metadata:
  name: %s
spec:
  type: Edge' > %s`
)

func CreateEdgeNpTemplate() string {
	return edgeTemplate
}
