## vHive CI guide

## deploying GitHub self-hosted runners
* Create Create GitHub personal access token for your repo with [these](https://github.com/myoung34/docker-github-actions-runner#create-github-personal-access-token) scopes.
```bash
# clone your repo
git clone <repo URL> && cd vhive
# setup host 
./scripts/github_runner/setup_runner_host.sh
# start integration test runners
./scripts/github_runner/start_runners.sh <num of runners> <Github repo URL> <Github Access key> integ
# start cri test runners
./scripts/github_runner/start_runners.sh <num of runners> <Github repo URL> <Github Access key> cri
```