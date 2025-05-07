package power_manager

import (
	"fmt"
	"os/exec"
)

func SetPowerProfileToNode(powerprofileName string, nodeName string, minFreq int64, maxFreq int64) error {
	// powerConfig
	command := fmt.Sprintf("kubectl apply -f - <<EOF\napiVersion: \"power.intel.com/v1\"\nkind: PowerConfig\nmetadata:\n  name: power-config\n  namespace: intel-power\nspec:\n powerNodeSelector:\n     kubernetes.io/os: linux\n powerProfiles:\n    - \"performance\"\nEOF")
	cmd := exec.Command("bash", "-c", command)
	_, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	// performanceProfile w freq
	command = fmt.Sprintf("kubectl apply -f - <<EOF\napiVersion: \"power.intel.com/v1\"\nkind: PowerProfile\nmetadata:\n  name: %s\n  namespace: intel-power\nspec:\n  name: \"%s\"\n  max: %d\n  min: %d\n  shared: true\n  governor: \"performance\"\nEOF", powerprofileName, powerprofileName, minFreq, maxFreq)
	cmd = exec.Command("bash", "-c", command)

	_, err = cmd.CombinedOutput()
	if err != nil {
		return err
	}

	// apply to node
	command = fmt.Sprintf("kubectl apply -f - <<EOF\napiVersion: \"power.intel.com/v1\"\nkind: PowerWorkload\nmetadata:\n  name: %s-%s-workload\n  namespace: intel-power\nspec:\n  name: \"%s-%s-workload\"\n  allCores: true\n  powerNodeSelector:\n    kubernetes.io/hostname: %s\n  powerProfile: \"%s\"\nEOF", powerprofileName, nodeName, powerprofileName, nodeName, nodeName, powerprofileName)
	cmd = exec.Command("bash", "-c", command)

	_, err = cmd.CombinedOutput()
	if err != nil {
		return err
	}
	return nil
}
