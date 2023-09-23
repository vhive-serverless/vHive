package template

const (
	masterNpTemplate = `apiVersion: apps.openyurt.io/v1beta1
kind: NodePool
metadata:
  name: master 
spec:
  type: Cloud`
)

func GetMasterNpTemplateConfig() string {
	return masterNpTemplate
}
