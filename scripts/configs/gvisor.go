package configs

type VHiveConfigStruct struct {
	GVisorVHiveBranch       string
	GVisorVHiveRepoUrl      string
	FirecrackerVHiveBranch  string
	FirecrackerVHiveRepoUrl string
}

var VHive = VHiveConfigStruct{
	GVisorVHiveBranch:       "main",
	GVisorVHiveRepoUrl:      "https://github.com/vhive-serverless/vHive.git",
	FirecrackerVHiveBranch:  "main",
	FirecrackerVHiveRepoUrl: "https://github.com/vhive-serverless/vHive.git",
}
