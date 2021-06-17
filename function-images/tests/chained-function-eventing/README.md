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
1. Download the programs in `bin/`:
   ```bash
   git lfs pull
   ```
1. Apply the configuration
   ```bash
   ./function-images/tests/chained-function-eventing/manifests/apply.sh
   ```

### Invoking
**On the master node**, execute the following instructions below:
1. Start a TimestampDB experiment:
   ```bash
   ./bin/tscli 10.96.0.84:80 start ./function-images/tests/chained-function-eventing/tscli.json
   ```
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
3. Inspect the **timestampdb** records:
   ```bash
   ./bin/tscli 10.96.0.84:80 end results.json
   ```

   **Sample Output:**
   ```
   INVOCATION {b8a155f8-343a-4c0b-a1bc-f087d9c4a89b}
   ==================================================
   Id       : b8a155f8-343a-4c0b-a1bc-f087d9c4a89b
   InvokedOn: 2021-06-17T19:41:56+0000
   Duration : 0s
   Status   : COMPLETED
   
   
   INVOCATION {8348ad0e-03cd-4247-86dc-81d6be805eba}
   ==================================================
   Id       : 8348ad0e-03cd-4247-86dc-81d6be805eba
   InvokedOn: 2021-06-17T19:41:58+0000
   Duration : 0s
   Status   : COMPLETED
   
   
   INVOCATION {4f01b213-2c8f-4770-87e9-cc493df28114}
   ==================================================
   Id       : 4f01b213-2c8f-4770-87e9-cc493df28114
   InvokedOn: 2021-06-17T19:41:59+0000
   Duration : 0s
   Status   : COMPLETED
   
   
   2021/06/17 15:42:04 END OK.
   ```
   - For machine-readable output, see `results.json`.
   - To inspect the logs of **relay**:
     ```bash
     kubectl logs -n chained-functions-eventing svc/relay
     ```
   - To inspect the logs of **timeseriesdb**:
     ```bash
     kubectl logs -n chained-functions-eventing svc/timeseriesdb
     ```

### Deleting
**On the master node**, execute the following instructions below:
1. Delete the configuration
   ```bash
   ./function-images/tests/chained-function-eventing/manifests/delete.sh
   ```
