package configs

import (
	"encoding/json"
	"io"
	"os"
	"path"

	utils "github.com/vhive-serverless/vHive/scripts/utils"
)

// Decode specific config files (JSON format)
func DecodeConfig(configFilePath string, configStruct interface{}) error {
	// Open & read the config file
	configFile, err := os.Open(configFilePath)
	if err != nil {
		return err
	}
	defer configFile.Close()

	// Read file content
	configContent, err := io.ReadAll(configFile)
	if err != nil {
		return err
	}

	// Decode json into struct
	err = json.Unmarshal(configContent, configStruct)

	return err

}

// Load knative config files
func (knative *KnativeConfigStruct) LoadConfig() error {
	var err error
	// Check config directory
	if len(VHive.VHiveSetupConfigPath) == 0 {
		VHive.VHiveSetupConfigPath, err = utils.GetVHiveFilePath("configs/setup")
		if err != nil {
			utils.CleanEnvironment()
			os.Exit(1)
		}
	}
	// Get the (absolute) path of the config file
	configFilePath := path.Join(VHive.VHiveSetupConfigPath, "knative.json")

	// Decode json into struct
	err = DecodeConfig(configFilePath, knative)

	return err

}

// Load kubernetes config files
func (kube *KubeConfigStruct) LoadConfig() error {
	// Get the (absolute) path of the config file
	configFilePath := path.Join(VHive.VHiveSetupConfigPath, "kube.json")

	// Decode json into struct
	err := DecodeConfig(configFilePath, kube)

	return err
}

// Load system config files
func (system *SystemEnvironmentStruct) LoadConfig() error {
	// Get the (absolute) path of the config file
	configFilePath := path.Join(VHive.VHiveSetupConfigPath, "system.json")

	// Decode json into struct
	err := DecodeConfig(configFilePath, system)

	return err
}

// Load vHive config files
func (vhive *VHiveConfigStruct) LoadConfig() error {
	// Get the (absolute) path of the config file
	configFilePath := path.Join(VHive.VHiveSetupConfigPath, "vhive.json")

	// Decode json into struct
	err := DecodeConfig(configFilePath, vhive)

	return err

}

const (
	Version = "0.2.4b" // Version Info
)
