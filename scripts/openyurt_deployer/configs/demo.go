package configs

type DemoEnvironment struct {
	CloudYamlFile      string
	EdgeYamlFile       string
	CloudBenchYamlFile string
	EdgeBenchYamlFile  string
	YurtAppSetYamlFile string
	CloudPoolName      string
	EdgePoolName       string
}

var Demo = DemoEnvironment{
	CloudYamlFile:      "cloud.yaml",
	EdgeYamlFile:       "edge.yaml",
	CloudBenchYamlFile: "cloud-bench.yaml",
	EdgeBenchYamlFile:  "edge-bench.yaml",
	YurtAppSetYamlFile: "yurt.yaml",
	CloudPoolName:      "cloud",
	EdgePoolName:       "edge",
}
