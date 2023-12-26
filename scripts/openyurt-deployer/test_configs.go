package main

// criteria table for testing ParsingNodeDependencyVersion
var criteriaTable = map[string]string{
	"Golang":     "1.19.10",
	"containerd": "1.6.18",
	"runc":       "1.1.4",
	"CNI":        "1.2.0",
}

// mock node info
var mockNodesInfo = NodesInfo{
	Master: "runner@127.0.0.1",
}

// data for github runner to ssh
var githubRunner = "runner@127.0.0.1"

// mock node data structure
var mockNode = Node{
	Name:     githubRunner,
	Client:   SetupSSHConn(githubRunner),
	NodeRole: "master",
	Configs: &NodeConfig{
		System:  System,
		Kube:    Kube,
		Knative: Knative,
		Yurt:    Yurt,
		Demo:    Demo,
	},
}
