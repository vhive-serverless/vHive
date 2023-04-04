# vHive CI guide

## Deploying GitHub self-hosted runners manually

### Deploying integration tests workflow runners
1. Create a GitHub personal access token (PAT) for your repo with [these](https://github.com/myoung34/docker-github-actions-runner#create-github-personal-access-token) scopes.

2. Clone the repository.

```bash
git clone https://github.com/vhive-serverless/vhive && cd vhive
```

3. Setup the runner host:

```bash
./scripts/github_runner/setup_integ_runners_host.sh
```

4. Setup the desired number of runners:

```bash
./scripts/github_runner/setup_integ_runners.sh <num of runners> <GitHub Account: e.g., vhive-serverless> <Github PAT> [restart]
```

The `restart` flag indicates whether runners are already setup on this host and a cleanup needs to be performed first.

### Deploying CRI tests workflow runners

1. Clone the repository.

```bash
git clone https://github.com/vhive-serverless/vhive && cd vhive
```

2. Setup the runner for the desired sandbox:

```bash
./scripts/github_runner/setup_bare_metal_runner.sh <GitHub Account: e.g., vhive-serverless> <Github PAT> <firecracker|gvisor> [restart]
```

The `restart` flag indicates whether a runner is already setup on this host and a reboot is needed.

## Deploying GitHub self-hosted runners automatically

1. Clone the repository.

```bash
git clone https://github.com/vhive-serverless/vhive && cd vhive
```

2. Create a runner deployer configuration (by default it assumed to be `./scripts/github_runner/conf.json`), for instance:

```json
{
  "ghOrg": "curiousgeorgiy",
  "ghPat": "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",
  "hostUsername": "glebedev",
  "runners": {
    "pc85.cloudlab.umass.edu": {
      "type": "cri",
      "sandbox": "firecracker"
    },
    "pc71.cloudlab.umass.edu": {
      "type": "integ",
      "num": 2
    }
  }
}
```

3. Build and run the runner deployer.

```bash
cd ./scripts/github_runner && go build && ./runner_deployer
```

## Clean up for restart
```bash
# list all kind clusters
kind get clusters 
# delete a cluster 
kind delete cluster --name <name>
# list remaining docker containers
docker ps
# stop a docker container
docker container stop <container ID>
#docker remove container
docker container rm <container ID>
```
