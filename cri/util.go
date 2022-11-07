package cri

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	criapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// (Key, Value) pair is mapped to a 'Key=Value' entry.
func ToStringArray(envVariables []*criapi.KeyValue) []string {
	result := make([]string, len(envVariables))

	for _, kv := range envVariables {
		env := fmt.Sprintf("%s=%s", kv.GetKey(), kv.GetValue())
		result = append(result, env)
	}

	log.Debugf("Converted '%v' to '%v'\n", envVariables, result)
	return result
}
