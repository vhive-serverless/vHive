package template

const (
	restartPodsShellTemplate = `existingPods=$(kubectl get pod -A -o wide | grep %s)
originalIFS=${IFS}
IFS=$'\n'
while read -r pod
do
	if [ -z "$(echo ${pod} | sed -n "/.*yurt-hub.*/p")" ]; then
		podNameSpace=$(echo ${pod} | sed -n "s/\s*\(\S*\)\s*\(\S*\).*/\1/p")
		podName=$(echo ${pod} | sed -n "s/\s*\(\S*\)\s*\(\S*\).*/\2/p")
		echo "${podNameSpace} ${podName}"
	fi
done <<< ${existingPods}
IFS=${originalIFS}`
)

func GetRestartPodsShell() string {
	return restartPodsShellTemplate
}
