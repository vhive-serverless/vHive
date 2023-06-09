package configs

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path"
)

func DecodeConfig(configFilePath string, configStruct interface{}) error {
	// Open & read the config file
	configFile, err := os.Open(configFilePath)
	if err != nil {
		return err
	}
	defer configFile.Close()

	// Read file content
	configContent, err := ioutil.ReadAll(configFile)
	if err != nil {
		return err
	}

	// Decode json into struct
	err = json.Unmarshal(configContent, configStruct)

	return err

}

func (knative *KnativeConfigStruct) LoadConfig() error {
	// Get the (absolute) path of the config file
	configFilePath := path.Join(VHive.VHiveSetupConfigPath, "knative.json")

	// Decode json into struct
	err := DecodeConfig(configFilePath, knative)

	return err

}

func (kube *KubeConfigStruct) LoadConfig() error {
	// Get the (absolute) path of the config file
	configFilePath := path.Join(VHive.VHiveSetupConfigPath, "kube.json")

	// Decode json into struct
	err := DecodeConfig(configFilePath, kube)

	return err
}

func (system *SystemEnvironmentStruct) LoadConfig() error {
	// Get the (absolute) path of the config file
	configFilePath := path.Join(VHive.VHiveSetupConfigPath, "system.json")

	// Decode json into struct
	err := DecodeConfig(configFilePath, system)

	return err
}

func (vhive *VHiveConfigStruct) LoadConfig() error {
	// Get the (absolute) path of the config file
	configFilePath := path.Join(VHive.VHiveSetupConfigPath, "vhive.json")

	// Decode json into struct
	err := DecodeConfig(configFilePath, vhive)

	return err

}
