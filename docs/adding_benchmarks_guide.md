# Adding Benchmarks to vHive/Knative and Stock Knative
When creating a benchmark which is composed of multiple functions, you can choose to compose your 
functions with synchronous calls (where a caller waits for the callee) or asynchronously (no 
waiting). The Knative programming model can support both of these through its _Serving_ and 
_Eventing_ modules, and so if you have a workload which you would like to bring up to use with 
vHive or stock Knative you will need to choose between the two. Both approaches are described in 
this document along with details on any extra k8s or Knative services/manifests which are necessary
 for implementing your own benchmark.

Note that this is not a step-by-step guide, but rather a general rundown of the core steps that you
 will need to take in implementing your workload. This overview consists of general guidelines 
 which will apply to all implementations as well as sections dedicated specifically to the serving 
 and eventing approach.

## General Guidelines
Apart from using the serving or eventing Knative component, the process of composing your functions
 will also need to include support for a remote procedure call system such as `gRPC`, and a 
 container virtualisation solution such as `Docker`. Throughout this document we include references
 to examples of both serving and eventing using gRPC and Docker to implement a composition of 
 functions which runs with Knative on a k8s cluster, and as such we will give guidance for these 
 systems specifically, though similar factors should apply to any alternatives.

### RPC Implementation
Remote Procedure Call support allows your functions to communicate easily over a network. 
We use gRPC with protobuf in [our serving example](function-images/test/chained-function-serving). 
You can refer to the [gRPC tutorial](https://grpc.io/docs/languages/go/basics/) for details on 
usage of these systems.
 
First you should define a [proto file](https://developers.google.com/protocol-buffers/docs/proto3) 
which describes the services used to communicate between your functions. Use this to generate code 
with `protoc`, making sure that you have the appropriate plugins for your language (e.g. we use the
 Golang plugins as described [here](https://grpc.io/docs/languages/go/quickstart/). You can see an 
 example proto file and the generated code 
 [here](function-images/test/chained-function-serving/proto).

Within your functions you will need to support the appropriate proto service (i.e. implement a 
server or client using the generated proto code) by implementing the interface. Keep in mind that 
some functions such as the 
[producer in our serving example](function-images/test/chained-function-serving/producer/producer.go)
 will need to be both a server of one proto service and a client of another proto service 
 simultaneously. Refer to the [gRPC tutorial](https://grpc.io/docs/languages/go/basics/) for extra 
 detail.

### Dockerizing Functions
To deploy your functions on Knative you will need to package them as 
[docker containers](https://www.docker.com/resources/what-container). Each function which you want 
to deploy on your cluster will need to be a separate image. 

In our serving and eventing examples the 
[dockerfile](function-images/test/chained-function-serving/Dockerfile) uses target_arg arguments to
 work in tandem with the [Makefile](function-images/tests/chained-function-serving/Makefile) to 
 reduce repetition, but it is also fine to write simple separate dockerfiles for each function. You
 should push the images of your functions to docker hub to make them accessible on your cluster.

### Manifests - K8s, Knative, vHive
Your manifests are what is used to define your services. The specifics of what goes into a manifest
 and how many manifests you need depends on whether you use serving or eventing and the details on 
 both are given in their appropriate sections. 

For Knative services which use gRPC, such as the producer and consumer in both the 
[serving](function-images/test/chained-function-serving/service-producer.yaml) and 
[eventing examples](function-images/test/chained-function-eventing/manifests/2-ksvc.yaml), you will
 need to use the `h2c` port translation.

vHive manifests must follow a specific structure, and they rely on hosting a guest image on a stub 
image in order to work. See [this hello-world example](configs/knative_workloads/helloworld.yaml) 
for a typical vHive manifest. You must use `h2c` port translation, and specify your function as a 
guest image environment variable. The guest port variable must match the containerPort associated 
with the h2c, and the core container image must be `crccheck/hello-world:latest`. Because of these 
restrictions, you cannot use environment variables to interact with your function when using a 
vHive manifest, and this is also why we advise against relying on environment variables in general
 throughout the process of bringing up your workload.

### Tracing
Tracing is an optional system for gathering timing data in your cluster. Knative can easily be 
extended to export tracing data to a system such as [Zipkin](https://zipkin.io/), but you should 
also add opentelemetry/zipkin instrumentation to your functions to fully support the system. 

We have a simple [tracing utility](utils/tracing) to make the adding tracing to your workloads 
easier, though if you want to develop a more involved system you will need to refer to the 
[Opentelemetry documentation](https://opentelemetry.io/docs/). If you're using the tracing utility
 the process is very simple:
1. Initialise the tracer:
   ```go
   shutdown := tracing.InitBasicTracer(*url, "producer")
   defer shutdown()
   ```
   The url you provide should point to the zipkin span collector service, e.g. 
   `http://localhost:9411/api/v2/spans` if you're hosting zipkin locally (e.g. as a docker 
   container). See the [zipkin quickstart](https://zipkin.io/pages/quickstart).

   You can use the basic tracer for most applications, and in cases where you want to provide 
   additional attributes or you wish to specify a different sampling rate you can use 
   `InitCustomTracer`.
2. If your function is a server, include the server interceptor instrumentation during definition. 
   Example:
   ```go
   grpcServer := grpc.NewServer(grpc.UnaryInterceptor(otelgrpc.UnaryServerInterceptor()))
   ```
3. If your function is a client, include client interceptor instrumentation when connecting to a 
   server. Example:
   ```go
   conn, err := grpc.Dial(fmt.Sprintf("%v:%v", ps.consumerAddr, ps.consumerPort), 
                   grpc.WithInsecure(), grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()))
   ```

You can check the 
[producer in our serving example](/function-images/tests/chained-function-serving/producer/producer.go) 
for example usage of this utility for both server and client behaviour.

## Serving
To compose functions with serving we make use of the 
[Knative Serving component](https://knative.dev/docs/serving/). Each of our functions will 
effectively be a server for the functions that come before it, and a client of the functions that 
come after. For example in a simple chain of functions `A -> B -> C`, B would be a client of 
function C and a server for function A.

our example of function composition using serving can be found 
[here](function-images/test/chained-function-serving), and additional CI implementation which shows
 how this code is executed can be found 
[here](https://github.com/ease-lab/vhive/blob/main/.github/workflows/function-composition-bench.yml).
 This example implements a simple Client -> Producer -> Consumer function chain, whereby the client
 triggers the producer function to generate a random string, and the consumer consumes said string
 (by logging it to a file).

To deploy your workload with serving you will need to:
- Implement a remote procedure call system (e.g. `gRPC`)
- Dockerize your functions
- Write Knative manifests

### RPC Implementation
As mentioned in the general guidelines, your functions will communicate with RPC calls. In 
[our serving example](function-images/test/chained-function-serving) we use gRPC with protobuf to 
achieve this.

In serving, each "link" in your chain of functions needs to implement a protobuf service. For 
example in a chain `A -> B -> C` the A-B link will be one service and the B-C link will be a second
 service. You should define a 
 [proto file](https://developers.google.com/protocol-buffers/docs/proto3) for these services, and 
 use it to generate code with `protoc`. You can see an example 
 [here](function-images/test/chained-function-serving/proto).

Implement the appropriate server or client service from the generated proto code in your functions 
and remember that some functions such as the 
[producer in our example](function-images/test/chained-function-serving/producer/producer.go) will 
need to be both a server of one proto service and a client of another proto service simultaneously.

### Dockerizing Functions
Every function needs to be built as a docker image. Refer to the general guidelines.

### Knative Manifests
You will need a Knative service definition for each of your functions. Refer to the 
[Knative docs](https://knative.dev/docs/serving/getting-started-knative-app/) and see 
[our example manifests](function-images/tests/chained-function-serving/service-producer.yaml) for 
support. Remember to include the `h2c` port translation if you used gRPC. We recommend avoiding the
 usage of environment variables if you want to work with vHive.

### Deployment
We recommend following the [vHive developers guide](docs/developers_guide.md) to set up your 
workload. When deploying your functions from your Knative manifests you can make sure that they are
 working with `kn service list` and `kn service describe <name>`.

If Knative is struggling to make revisions of your pods (e.g. your service is labeled as 
unschedulable) you might be using the wrong ports in your function. Double-check your Knative 
manifests and your function code. You should be serving on port 80 by default, or checking the 
$PORT environment variable which is will be set by Knative when deploying your function.

If you notice that some pods are stuck on pending (e.g. by using `kubectl get pods -A`) you might 
have exhausted system resources. This can occur in situations where you have too many pods or 
containers running on your system (e.g. if you work from within `kind` containers on cloudlab), or 
when using default github runners for your workflows.

## Eventing
You can also make use of the [Knative Eventing component](https://knative.dev/docs/eventing/) to 
compose functions. There are two different eventing approaches in Knative, and we will use the 
Broker-Trigger model as advised---see [Knative Eventing Primer](docs/knative/evening.md) for an 
introduction to Knative Eventing.

Whilst it is possible to expose our broker directly to the outside world, it makes more sense to 
have a service in front of it, for tasks like authentication, authorization, validation, and also 
to abstract the particular broker implementation away.

An example of function composition using eventing can be found 
[here](function-images/test/eventing). This example implements a simple Client (grpcurl) -> 
Producer -> Consumer function chain, whereby the client triggers the producer function to generate
 an event, and the consumer consumes said event. The CI workflow for this example can be found 
 [here](https://github.com/ease-lab/vhive/blob/main/.github/workflows/function-composition-bench.yml),
 showing how the example can be deployed. 

In general, to deploy your workload with eventing you will need to:
- Implement a **producer** server that processes incoming requests and raises corresponding events
- Implement event **consumers** that handle the events that you are interested in
- Dockerize your functions
- Write Knative manifests for your services and supporting components (e.g. Triggers, SinkBindings,
 etc.)

### Knative Manifests
Below presented Knative manifests of some components that we believe might be helpful to explain, 
namely:
- SinkBinding
- Trigger

#### SinkBinding
**Example:**
```yaml
apiVersion: sources.knative.dev/v1
kind: SinkBinding
metadata:
  name: my-sinkbinding
  namespace: my-namespace-a
spec:
  subject:
    apiVersion: serving.knative.dev/v1
    kind: Service
    name: my-service
    namespace: my-namespace-b
  sink:
    ref:
      apiVersion: eventing.knative.dev/v1
      kind: Broker
      name: my-broker
      namespace: my-namespace-c
```

- A SinkBinding is a component that injects `K_SINK` and some other environment variables to 
configure services dynamically on runtime---such services can use the injected environment 
variables to address the Broker, the Channel, or even another Service (all called a _"sink"_) to 
send CloudEvents.
- A SinkBinding, the subject ("sender"), and the sink ("receiver") may exist in different 
namespaces.

#### Trigger
**Example:**
```yaml
apiVersion: eventing.knative.dev/v1
kind: Trigger
metadata:
  name: my-trigger
  namespace: my-namespace
spec:
  broker: my-broker
  filter:
    attributes:
      type: my-cloudevent-type
      source: my-cloudevent-source
      my-custom-extension: my-custom-value
  subscriber:
    ref:
      apiVersion: serving.knative.dev/v1
      kind: Service
      name: my-kservice
      namespace: my-namespace
```

- A Trigger is a link between a broker and a _subscriber_ that relays the incoming CloudEvents to 
the broker after filtering them on certain attributes, commonly `type` and `source` but possibly 
also any other 
[attribute extensions](https://github.com/cloudevents/spec/blob/master/primer.md#cloudevent-attributes).
- A Trigger must be in the same namespace with the broker it is attached to, but can relay 
CloudEvents to any 
[_addressable_](https://github.com/knative/specs/blob/main/specs/eventing/interfaces.md#addressable)
 _subscriber_ in any namespace.


### Deployment
- While deploying a SinkBinding, best wait until the _sink_ ("receiver") is ready and thus has an 
address that the SinkBinding can use.
    - Services that depend during initialization on the environment variables injected by the 
    SinkBinding might fail (repeatedly) upon deployment until a relevant SinkBinding is applied; 
    that is normal.
- While deploying a Trigger, best wait until both the _broker_ and the _subscriber_ ("receiver") 
are ready.

## Other recommendations
### Docker Compose
Optionally, you may choose to include a docker-compose manifest which helps with testing your 
deployment locally without Knative. All images deployed with docker-compose will be on the same 
network so they can communicate easily and you can use the names of services as addresses since 
they are translated automatically by docker. Make sure to expose a port so that your client can 
communicate with the initial function in your workload. See 
[our serving docker-compose](function-images/tests/chained-function-serving/docker-compose.yml) for
 an example.
