# Knative Eventing Tutorial
See [Knative Eventing Tutorial](/docs/knative/eventing.md) for a primer on Knative Eventing.

Tested on a **CUSTOM** single-node stock Knative setup, [see vHive Developer's Guide](https://github.com/ease-lab/vhive/blob/main/docs/developers_guide.md#testing-stock-knative-images)---uses [knative/client@0c6ef82](https://github.com/knative/client/commit/0c6ef82a56654c9c7677a3d1059f74440324b27c).

## Kubernetes Based
### Starting
**On the master node**, execute the following instructions below:
1. Apply the configuration
   ```bash
   kubectl apply --filename config-k8s.yaml
   ```

### Invoking
You can interact with the pipeline either (A) from your local machine, or (B) from the master node.

#### Option A. From Your Local Machine
**On your local machine**, execute the following instructions below: 
1. Install [grpcurl](https://github.com/fullstorydev/grpcurl/releases) if not already installed:
   ```bash
   wget -qO- https://github.com/fullstorydev/grpcurl/releases/download/v1.8.1/grpcurl_1.8.1_linux_x86_64.tar.gz | sudo tar -C /usr/bin/ -xz grpcurl 
   ```
2. Make a gRPC request:
   ```bash
   grpcurl -d '{"celsius": 97, "provider": "aws", "zone": "us-east-1", "machine": "monet"}' -plaintext <SERVER_HOSTNAME>:30002 TempReader.ReadTemp
   ```

   `<SERVER_HOSTNAME>` is the hostname/IP address of one of your worker nodes, or in the single-node setup the server itself; for instance, `pc02.cloudlab.umass.edu`.

#### Option B. From The Master Node
**On the master node**, execute the following instructions below:
1. Install [grpcurl](https://github.com/fullstorydev/grpcurl/releases) if not already installed:
   ```bash
   wget -qO- https://github.com/fullstorydev/grpcurl/releases/download/v1.8.1/grpcurl_1.8.1_linux_x86_64.tar.gz | sudo tar -C /usr/bin/ -xz grpcurl 
   ```
2. Get the **cluster IP** of `ease-pipeline-server`:
   ```bash
   kubectl get Service/ease-pipeline-server
   ```
3. Make a gRPC request:
   ```bash
   grpcurl -d '{"celsius": 97, "provider": "aws", "zone": "us-east-1", "machine": "monet"}' -plaintext <CLUSTER IP>:80 TempReader.ReadTemp
   ```

### Inspecting
**On the master node**, execute the following instructions below:
1. Inspect the **server** logs:
   ```bash
   kubectl logs Service/ease-pipeline-server
   ```
2. Inspect the **`temp`-consumer** logs:
   ```bash
   kubectl logs Service/ease-pipeline-temp-consumer
   ```
3. Inspect the **`overheat`-consumer** logs:
   ```bash
   kubectl logs Service/ease-pipeline-overheat-consumer
   ```

### Deleting
**On the master node**, execute the following instructions below:
1. Delete the configuration
   ```bash
   kubectl delete --filename config-k8s.yaml
   ```

## Knative Based
### Starting
**On the master node**, execute the following instructions below:
1. Apply the configuration
   ```bash
   kubectl apply --filename config-kn.yaml
   ```

### Invoking
**On the master node**, execute the following instructions below:
1. Install [grpcurl](https://github.com/fullstorydev/grpcurl/releases) if not already installed:
   ```bash
   wget -qO- https://github.com/fullstorydev/grpcurl/releases/download/v1.8.1/grpcurl_1.8.1_linux_x86_64.tar.gz | sudo tar -C /usr/bin/ -xz grpcurl 
   ```
2. Wait until all services are ready:
   ```bash
   watch kubectl get ksvc
   ```
   - Press <kbd>Ctrl</kbd>+<kbd>C</kbd> to exit.
2. Get the **URL** of `ease-pipeline-server`:
   ```bash
   kubectl get ksvc/ease-pipeline-server
   ```
2. Make a gRPC request, **stripping the protocol part** (i.e., `http://`) of the URL:
   ```bash
   grpcurl -d '{"celsius": 97, "provider": "aws", "zone": "us-east-1", "machine": "monet"}' -plaintext <URL>:80 TempReader.ReadTemp
   ```

### Inspecting
> **BEWARE:**
>
> Because Knative scales down to zero when idle, logs disappear if you wait too long after invoking. Invoke again before inspecting the logs, and proceed quickly.

**On the master node**, execute the following instructions below:
1. Inspect the **server** logs:
   ```bash
   kubectl logs -c user-container -l serving.knative.dev/service=ease-pipeline-server
   ```
   - **Ignore** the following error:
      ```
      Failed to process env var: required key K_SINK missing value
      ```
2. Inspect the **`temp`-consumer** logs:
   ```bash
   kubectl logs -c user-container -l serving.knative.dev/service=ease-pipeline-temp-consumer
   ```
3. Inspect the **`overheat`-consumer** logs:
   ```bash
   kubectl logs -c user-container -l serving.knative.dev/service=ease-pipeline-overheat-consumer
   ```
   - `overheat`-consumer is triggered only for temperatures > 80 °C.

### Deleting
**On the master node**, execute the following instructions below:
1. Delete the configuration
   ```bash
   kubectl delete --filename config-kn.yaml
   ```
