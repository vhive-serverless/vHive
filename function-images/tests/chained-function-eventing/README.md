# Knative Eventing μBenchmarks

## Design
We present a very simple _hello world_ pipeline where
- **producer** accepts incoming `Greeter` gRPC requests from outside world;
- **consumer** digests `greeting` events produced by the _producer_.

Both services log extensively for debugging and/or testing.

### Manifests
Manifests are split into separate files and are enumerated in the order of their dependencies. Using the helper script `apply.sh`, one can setup the pipeline correctly whilst waiting for each component to get ready before applying the others that depend on it. This is especially important on CI pipelines, where components take longer to get ready.

## Running Manually
### Starting
**On the master node**, execute the following instructions below:
1. Apply the configuration
   ```bash
   ./function-images/tests/chained-function-eventing/manifests/apply.sh
   ```

### Invoking
**On the master node**, execute the following instructions below:
1. Make a gRPC request:
   ```bash
   ./bin/grpcurl -d '{"name": "Bora"}' -plaintext producer.chained-functions-eventing.192.168.1.240.sslip.io:80 helloworld.Greeter.SayHello
   ```

### Inspecting
> **BEWARE:**
>
> Because Knative scales down to zero when idle, logs disappear if you wait too long after invoking. Invoke again before inspecting the logs, and proceed quickly.

**On the master node**, execute the following instructions below:
1. Inspect the **producer** logs:
   ```bash
   kubectl logs -n chained-functions-eventing -c user-container -l serving.knative.dev/service=producer
   ```
    - **Ignore** the following error:
       ```
       Failed to process env var: required key K_SINK missing value
       ```
2. Inspect the **consumer** logs:
   ```bash
   kubectl logs -n chained-functions-eventing -c user-container -l serving.knative.dev/service=consumer
   ```

### Deleting
**On the master node**, execute the following instructions below:
1. Delete the configuration
   ```bash
   ./function-images/tests/chained-function-eventing/manifests/delete.sh
   ```
