package configs

type VHiveConfigStruct struct {
	FirecrackerKernelImgDownloadUrl string
	StargzVersion                   string
	VHiveRepoPath                   string
	VHiveRepoBranch                 string
	VHiveRepoUrl                    string
	VHiveSetupConfigPath            string
}

var VHive = VHiveConfigStruct{
	VHiveRepoPath:        "",
	VHiveRepoBranch:      "main",
	VHiveRepoUrl:         "https://github.com/vhive-serverless/vHive.git",
	VHiveSetupConfigPath: "",
}
