// MIT License
//
// Copyright (c) 2023 Haoyuan Ma and vHive team
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package configs

import (
	"encoding/json"
	"io"
	"os"
	"path"
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
	// Get the (absolute) path of the config file
	configFilePath := path.Join(VHive.VHiveSetupConfigPath, "knative.json")

	// Decode json into struct
	err := DecodeConfig(configFilePath, knative)

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
