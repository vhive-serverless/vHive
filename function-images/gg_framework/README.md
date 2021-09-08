# gg with vHive
[`gg`](https://www.usenix.org/conference/atc19/presentation/fouladi) is a framework and a set of command-line tools that helps people execute everyday applications—e.g., software compilation, unit tests, video encoding, or object recognition—using thousands of parallel threads on a cloud-functions service to achieve near-interactive completion time.

## Setting up `gg` to run with vHive
1. Clone the `gg` repository: `git clone https://github.com/StanfordSNR/gg.git`
2. Follow the instructions in `gg`'s [README](https://github.com/StanfordSNR/gg#readme) to build `gg` and set up your environment.
Stop when you complete the [Environment Variables](https://github.com/StanfordSNR/gg#environment-variables) setup.

   `gg`'s execution binary used with vHive was built with the master branch, commit `04063e5`. However, this binary is independent of the compute backend, including vHive (i.e., it is the same one `gg` uses for all of its compute backends).

    **Important note**: To avoid protobuf-related compilation issues, `gg` must currently be built BEFORE you build and set up vHive. Therefore, please finish this step before building and setting up vHive (Step 3).

3. Build and set up vHive using the [quickstart guide](https://github.com/ease-lab/vhive/blob/main/docs/quickstart_guide.md).
4. From the root directory of vHive, deploy the `gg` function to vHive using:
    ```
    kubectl apply --filename configs/knative_workloads/gg_framework.yaml
    ```
    or
    ```
    kn service apply gg-framework -f configs/knative_workloads/gg_framework.yaml
    ```

You can check that the `gg` function was deployed successfully by using `kubectl get pods -A` and checking to see that the `gg` pod is running with 2/2 containers.

## Running `gg` applications
Since `gg` decouples the frontend and backend, no changes are needed to run `gg` applications with vHive!
To run with vHive, you first need to get the URL of the deployed function.
You then pass this URL to the engine parameter when calling `gg force`.
Here is how to do that assuming we are trying to force a file called *test* with two-way parallelism:
```
kn service describe gg-framework -o url # Output of the form: http://gg-framework.default.123.456.7.890.sslip.io
gg force --jobs 2 --engine vhive=http://gg-framework.default.123.456.7.890.sslip.io test
```

Other parameters to `gg force` can be reviewed by simply calling `gg force` with no parameters.
However, the only required one to work with vHive is `--engine vhive=<url>`.

Any of `gg`'s supported storagge backends (e.g., Amazon S3 and Google Cloud Storage) work with vHive. Please see `gg`'s [README](https://github.com/StanfordSNR/gg#readme) for how to set the _GG_STORAGE_URI_ variable.

### Examples
`gg` includes a number of [examples](https://github.com/StanfordSNR/gg/tree/master/examples) which can be run with vHive as the backend.
