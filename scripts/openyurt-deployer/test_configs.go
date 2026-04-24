package main

// criteria table for testing ParsingNodeDependencyVersion
var criteriaTable = map[string]string{
	"Golang":     "1.26.2",
	"containerd": "2.2.3",
	"runc":       "1.3.5",
	"CNI":        "1.7.1",
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
