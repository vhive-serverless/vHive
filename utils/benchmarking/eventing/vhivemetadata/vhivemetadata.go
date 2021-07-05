package vhivemetadata

import (
	"encoding/json"
	"time"

	ctrdlog "github.com/containerd/containerd/log"
	"github.com/sirupsen/logrus"
)

type vHiveMetadata struct {
	WorkflowId string `json:"WorkflowId"`
	InvocationId string `json:"InvocationId"`
	InvokedOn time.Time `json:"InvokedOn"`
}

func GetWorkflowId(d []byte) string {
	return unmarshalVHiveMetadata(d).WorkflowId
}

func GetInvocationId(d []byte) string {
	return unmarshalVHiveMetadata(d).InvocationId
}

func GetInvokedOn(d []byte) time.Time {
	return unmarshalVHiveMetadata(d).InvokedOn
}

func unmarshalVHiveMetadata(d []byte) (vhm vHiveMetadata) {
	if err := json.Unmarshal(d, &vhm); err != nil {
		logrus.Fatal("failed to unmarshal vhivemetadata", err)
	}
	return
}

func MakeVHiveMetadata(WorkflowId, InvocationId string, InvokedOn time.Time) []byte {
	return marshalVHiveMetadata(vHiveMetadata{
		WorkflowId:   WorkflowId,
		InvocationId: InvocationId,
		InvokedOn:    InvokedOn,
	})
}

func marshalVHiveMetadata(vhm vHiveMetadata) []byte {
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
		logrus.Fatal("failed to marshal vHiveMetadata", err)
	}
	return d
}

