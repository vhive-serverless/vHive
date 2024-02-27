# How to collect and extract vHive logs to local machine


This guide describes how to collect and extract logs in an N-node vHive serverless cluster with Firecracker MicroVMs.

Kubelet has its own logs, which can have different verbosity levels. This verbosity can be controlled by the `LogVerbosity` parameter in `configs/setup/system.json`. In addition, this parameter changes the sizes of per-container logs. By default, this parameter is set to 0 for performance reasons.

There are a couple of ways to gather and extract logs for vHive to your local machine, whether it is on a single-node cluster or a multi node one.
We will present one method to do it.
Firstly, if you follow the steps from the [Quickstart guide], logs should already be generated in the `/tmp/vhive-logs` folder at different steps in the workflow.
However, sometimes because of running some commands with `screen`, the log folder ends up empty.
To mitigate this, you can run the same commands using different `tmux panes`.

>
> **Note:** `tmux` serves the same role as `screen`, being a terminal multiplexer offering similar features.
> 

The following section will describe how to set up a Serverless (Knative) Cluster using `tmux`.
This section follows the steps from the Quickstart guide with minor modifications for the log generation and running commands without `screen`.
We present how to set up a multi-node cluster, however, the same modifications can be used to generate and extract logs from a single-node cluster.

   > **Beware:**
   > The following commands are meant to be run in a shell of type `bash` as mentioned in the [Quickstart guide][shell-type].
   > Running them with a different kind of shell interpreter can lead to undefined behaviour.
   >


### 1. Setup All Nodes
**On each node (both master and workers)**, execute the following instructions **as a non-root user with sudo rights** using **bash**:
1. Clone the vHive repository, change your working directory to the root of the repository and create a directory for vHive logs:
   ```bash
   git clone --depth=1 https://github.com/vhive-serverless/vhive.git && cd vhive && mkdir -p /tmp/vhive-logs
   ```

2. Build `setup_tool`

    ```bash
    ./scripts/install_go.sh; source /etc/profile
    pushd scripts && go build -o setup_tool && popd && mv scripts/setup_tool .
    ```

3. Run the node setup script:
    ```bash
    ./setup_tool setup_node REGULAR
    ```
    > **BEWARE:**
    >
    > This script can print `Command failed` when creating the devmapper at the end. This can be safely ignored.

### 2. Setup Worker Nodes
**On each worker node**, execute the following instructions **as a non-root user with sudo rights** using **bash**:
1. Run the script that sets up the kubelet:
    ```bash
    ./setup_tool setup_worker_kubelet
    ```
    
2. Open a new `tmux session` in detached mode and start `containerd` in the detached session:
    ```bash
    tmux new -s ctrd -d && tmux send-keys -t ctrd "sudo containerd 2>&1 | tee /tmp/vhive-logs/containerd_log.txt" Enter
    ```
    
    > **Note:**
    >
    > The logs from `containerd` will be generated at this path `/tmp/vhive-logs/containerd_log.txt`. You can change the log's file name or directory.
    > The same applies for all the log files used in the following commands.
    > Additionally, you will not see the output of the session unless you attach to it.
    > The `-s` flag specifies the name of the session, while `-d` specifies that we create the session without attaching to it.
    > If you want to attach to an existing session you can do so with the following command:
    > ```bash
    > tmux attach -t sessionName
    > ```
    > You can detach from a session with the following combination of keys: <kbd>Ctrl</kbd>+<kbd>B</kbd> then <kbd>d</kbd>.
    
3. Open a new `tmux session` in detached mode and start `firecracker-containerd`:
    ```bash
    tmux new -s firecracker -d && tmux send -t firecracker "sudo PATH=$PATH /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 2>&1 | tee /tmp/vhive-logs/firecracker-containerd_log.txt" Enter
    ```

4. Build vHive host orchestrator:
    ```bash
    source /etc/profile && go build
    ```
    
5. Open a new `tmux session` in detached mode and start `vHive`:
    ```bash
    tmux new -s vhive -d && tmux send -t vhive "sudo ./vhive -dbg -snapshots 2>&1 | tee /tmp/vhive-logs/vhive_log.txt" Enter
    ```
    > **Note:**
    >
    > By default, the microVMs are booted and vHive is started in debug mode enabled by `-dbg` flag.
    > Additionally, you can enable snapshots after the 2nd invocation of each function by adding the `-snapshots` flag.
    >
    > If `-snapshots` and `-upf` are specified, the snapshots are accelerated with the Record-and-Prefetch (REAP) technique that we described in our ASPLOS'21 paper ([extended abstract][ext-abstract], [full paper](papers/REAP_ASPLOS21.pdf)).

### 3. Configure Master Node
**On the master node**, execute the following instructions **as a non-root user with sudo rights** using **bash**:
1. Open a new `tmux session` in detached mode and start `containerd` in the detached session:
    ```bash
    tmux new -s ctrd -d && tmux send-keys -t ctrd "sudo containerd 2>&1 | tee /tmp/vhive-logs/containerd_log.txt" Enter
    ```
    
2. Run the script that creates the multinode cluster:
    ```bash
    ./setup_tool create_multinode_cluster firecracker
    ```
    
    > **BEWARE:**
    >
    > The script will ask you the following:
    > ```
    > All nodes need to be joined in the cluster. Have you joined all nodes? (y/n)
    > ```
    > **Leave this hanging in the terminal as we will go back to this later.**
    >
    > However, in the same terminal you will see a command in the following format:
    > ```
    > kubeadm join 128.110.154.221:6443 --token <token> \
    >     --discovery-token-ca-cert-hash sha256:<hash>
    > ```
    > Please copy both lines of this command.

### 4. Configure Worker Nodes
**On each worker node**, execute the following instructions **as a non-root user with sudo rights** using **bash**:

1. Add the current worker to the Kubernetes cluster, by executing the command you have copied in step (3.2) **using sudo**:
    ```bash
    sudo kubeadm join IP:PORT --token <token> --discovery-token-ca-cert-hash sha256:<hash> > >(tee -a /tmp/vhive-logs/kubeadm_join.stdout) 2> >(tee -a /tmp/vhive-logs/kubeadm_join.stderr >&2)
    ```
    > **Note:**
    >
    > On success, you should see the following message:
    > ```
    > This node has joined the cluster:
    > * Certificate signing request was sent to apiserver and a response was received.
    > * The Kubelet was informed of the new secure connection details.
    > ```

### 5. Finalise Master Node
**On the master node**, execute the following instructions **as a non-root user with sudo rights** using **bash**:

1. As all worker nodes have been joined, and answer with `y` to the prompt we have left hanging in the terminal.
2. As the cluster is setting up now, wait until all pods show as `Running` or `Completed`:
    ```bash
    watch kubectl get pods --all-namespaces
    ```

**Your Knative cluster is now ready for deploying and invoking functions.**

Now you can deploy and invoke the functions as instructed in the [Quickstart guide][deploy].
Furthermore, you should be able to see all your logs in the `/tmp/vhive-logs` directory.
Additionally, you can extract the logs from the nodes to your local machine.
Run this command on your local machine for each node of the cluster:
 ```bash
 scp -i PATH_TO_SSH_KEY -P 22 -r USERNAME@HOST_NAME:/tmp/vhive-logs PATH_TO_LOCAL_DIRECTORY/SUB_DIR
 ```
   > **Note:**
   >
   > PATH_TO_SSH_KEY represents the path to your ssh key used to connect to the cluster nodes.
   > 
   > USERNAME represents your username for connecting to the cluster nodes.
   >
   > HOST_NAME represents the address of the cluster node you want to extract the logs from.
   >
   > PATH_TO_LOCAL_DIRECTORY represents the local path where you would like to store the logs folder.
   >
   > SUB_DIR represents the sub-directory in which you will store the logs folder on your local host.
   > We need such a folder as the logs folder has the same path on every cluster node, in our case `/tmp/vhive-logs`.
   > In order to not overwrite the folder on the local host, we need to use a sub-directory.
   > **Be sure to change the sub-directory name when running the command for each node to avoid overwriting**.
   >
   > Additionally, you can change the location where the logs are generated in as instructed in previous steps.
   > Thus you will need to modify `/tmp/vhive-logs` in the command with the new log generation path.
  
  
[Quickstart guide]: https://github.com/vhive-serverless/vHive/blob/main/docs/quickstart_guide.md#ii-setup-a-serverless-knative-cluster
[shell-type]: https://github.com/vhive-serverless/vHive/blob/main/docs/quickstart_guide.md#3-cloudlab-deployment-notes
[deploy]: https://github.com/vhive-serverless/vHive/blob/main/docs/quickstart_guide.md#iv-deploying-and-invoking-functions-in-vhive
[ext-abstract]: https://asplos-conference.org/abstracts/asplos21-paper212-extended_abstract.pdf
