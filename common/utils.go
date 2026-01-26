package common

import (
	"context"
	"errors"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	criapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// (Key, Value) pair is mapped to a 'Key=Value' entry.
func WaitForConnectionReady(conn *grpc.ClientConn, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	conn.Connect()

	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			return nil
		}
		if !conn.WaitForStateChange(ctx, state) {
			err := ctx.Err()
			if err == nil {
				err = errors.New("connection closed or state change failed")
			}
			return fmt.Errorf("grpc connection failed: %w (last state: %s)", err, state)
		}
	}
}

// ToStringArray converts CRI KeyValue pairs to string array in 'Key=Value' format
func ToStringArray(envVariables []*criapi.KeyValue) []string {
	result := make([]string, len(envVariables))

	for _, kv := range envVariables {
		env := fmt.Sprintf("%s=%s", kv.GetKey(), kv.GetValue())
		result = append(result, env)
	}

	log.Debugf("Converted '%v' to '%v'\n", envVariables, result)
	return result
}
