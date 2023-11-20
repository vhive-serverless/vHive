package configs

type YurtEnvironment struct {
	HelmInstalled                   bool
	HelmPublicSigningKeyDownloadUrl string
	KustomizeInstalled              bool
	KustomizeScriptDownloadUrl      string
	MasterAsCloud                   bool
	WorkerNodeName                  string
	WorkerAsEdge                    bool
	Dependencies                    string
	YurtVersion                     string
}

var Yurt = YurtEnvironment{
	HelmInstalled:                   false,
	HelmPublicSigningKeyDownloadUrl: "https://baltocdn.com/helm/signing.asc",
	KustomizeInstalled:              false,
	KustomizeScriptDownloadUrl:      "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh",
	MasterAsCloud:                   true,
	WorkerNodeName:                  "",
	WorkerAsEdge:                    true,
	Dependencies:                    "",
	YurtVersion:                     "1.2.1",
}
