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

var Kube = KubeConfigStruct{
	K8sVersion:                "1.25.9",
	AlternativeImageRepo:      "",
	ApiserverAdvertiseAddress: "",
	PodNetworkCidr:            "192.168.0.0/16",
	PodNetworkAddonConfigURL:  "https://raw.githubusercontent.com/vhive-serverless/vHive/main/configs/calico/canal.yaml",
	ApiserverPort:             "6443",
	ApiserverToken:            "",
	ApiserverTokenHash:        "",
}
