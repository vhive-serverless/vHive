// MIT License
//
// Copyright (c) 2020 Plamen Petrov
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

package config

import (
	"encoding/json"
	"io/ioutil"

	"github.com/pkg/errors"
)

const (
	defaultConfigPath        = "/etc/fccd-orchestrator/config.json"
	defaultSnapshotter       = "devmapper"
	defaultLogLevel          = "Info"
	defaultSnapshotsEnabled  = false
	defaultUpfEnabled        = false
	defaultLazyModeEnabled   = false
	defaultMetricsEnabled    = false
	defaultSaveMemoryEnabled = false
	defaultServedThreshold   = 1000000
	defaultPinnedNum         = 0
)

// Config represents runtime configuration parameters
type Config struct {
	SaveMemoryEnabled bool   `json:"save_memory_enabled"`
	Snapshotter       string `json:"snapshotter"`
	LogLevel          string `json:"log_level"`
	SnapshotsEnabled  bool   `json:"snapshots_enabled"`
	UpfEnabled        bool   `json:"upf_enabled"`
	MetricsEnabled    bool   `json:"metrics_enabled"`
	ServedThreshold   uint64 `json:"served_threshold"`
	PinnedNum         int    `json:"pinned_num"`
	LazyModeEnabled   bool   `json:"lazy_mode_enabled"`
}

// LoadConfig loads configuration from JSON file at 'path'
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		path = defaultConfigPath
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read config from %q", path)
	}

	cfg := &Config{
		SaveMemoryEnabled: defaultSaveMemoryEnabled,
		Snapshotter:       defaultSnapshotter,
		LogLevel:          defaultLogLevel,
		SnapshotsEnabled:  defaultSnapshotsEnabled,
		UpfEnabled:        defaultUpfEnabled,
		MetricsEnabled:    defaultMetricsEnabled,
		ServedThreshold:   defaultServedThreshold,
		PinnedNum:         defaultPinnedNum,
		LazyModeEnabled:   defaultLazyModeEnabled,
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal config from %q", path)
	}
	return cfg, nil
}
