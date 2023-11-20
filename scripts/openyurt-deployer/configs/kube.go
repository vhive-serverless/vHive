package configs

type KubeConfigStruct struct {
	K8sVersion                string
	AlternativeImageRepo      string
	ApiserverAdvertiseAddress string
	PodNetworkCidr            string
	PodNetworkAddonConfigURL  string
	ApiserverPort             string
	ApiserverToken            string
	ApiserverTokenHash        string
}

var Kube = KubeConfigStruct{}
