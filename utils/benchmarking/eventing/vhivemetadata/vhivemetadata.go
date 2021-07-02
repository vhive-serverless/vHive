package vhivemetadata

import (
	"encoding/json"
	"time"

	ctrdlog "github.com/containerd/containerd/log"
)

type VHiveMetadata struct {
	WorkflowId string `json:"WorkflowId"`
	InvocationId string `json:"InvocationId"`
	InvokedOn time.Time `json:"InvokedOn"`
}

func UnmarshalVHiveMetadata(d []byte) (vhm VHiveMetadata, err error) {
	err = json.Unmarshal(d, &vhm)
	return
}

func MarshalVHiveMetadata(vhm VHiveMetadata) []byte {
	d, err := json.Marshal(struct {
		WorkflowId string `json:"WorkflowId"`
		InvocationId string `json:"InvocationId"`
		InvokedOn string `json:"InvokedOn"`
	}{
		WorkflowId: vhm.WorkflowId,
		InvocationId: vhm.InvocationId,
		InvokedOn: vhm.InvokedOn.Format(ctrdlog.RFC3339NanoFixed),
	})
	if err != nil {
		panic(err)
	}
	return d
}

