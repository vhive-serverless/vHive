package template

const (
	cloudTemplate = `echo 'apiVersion: apps.openyurt.io/v1beta1
kind: NodePool
metadata:
  name: %s 
spec:
  type: Cloud' > %s`
)

func CreateCloudNpTemplate() string {
	return cloudTemplate
}
