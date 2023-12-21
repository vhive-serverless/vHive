This is the deployment scripts of github self-hosted runners, used to execute some of the unit tests.

There are four self-hosted runners in total:
* cri-firecracker: Used for [firecracker cri tests](../../.github/workflows/integration_tests.yml)
* cri-gvisor: Used for [gvisor cri tests](../../.github/workflows/gvisor_cri_tests.yml)
* integ: Used for [integration tests](../../.github/workflows/integration_tests.yml)
* profile: Used for [profile unit tests](../../.github/workflows/unit_tests.yml), job: `profile-unit-test`

# Deploy Runners
Runners physical node configuration:
Four nodes with 4C-8G, 100GB storage. Suggested system image: `ubuntu-20.04-2nic`

How to deploy the four nodes:
1. Build the runner deployer
```
go build .
```
2. Modify the `conf.json`

Need to modify `conf.json`, the format is as following:
```
{
  "ghOrg": "<GitHub account>",
  "ghPat": "<GitHub PAT>",
  "hostUsername": "<username>",
  "runners": {
    "<hostname-1>": {
      "type": "cri",
      "sandbox": "firecracker"
    },
    "<hostname-2>": {
      "type": "cri",
      "sandbox": "gvisor",
    },
    "<hostname-3>": {
      "type": "integ",
      "num": 2,
      "restart": false
    },
    "<hostname-4>": {
        "type": "profile"
    }
  }
}
```

Note that in `conf.json`, for `ghOrg`, it's `vhive-serverless`, for `ghPat`, it should be your own account's Personal Access Token, as long as your account has the correct permissions for `vhive-serverless` org.
 
`<username>:<hostname-1/2/3/4>` is the ssh username and hostname, so if you use `SCSE` cloud nodes as runners, `<hostname-1/2/3/4>` should be their `ip` addresses.

After modifying this, deploy the runners remotely by running:
```
./deploy_runners
```

If it gives out error like `“dial unix: missing address”`, use:
```
eval `ssh-agent`
ssh-add ~/.ssh/<private_key>
```
Here `<private_key>` should be the key that has the ssh permission to all four runners, typically it's `id_rsa` 

# Restart Runners
On `SCSE` cloud, rebuild the four nodes and redeploy them.

# When Should Restart Runners
For firecracker and gvisor cri tests, when the test stuck in `helloworld is waiting for a Revision to be ready`
<img width="814" alt="bc67c34ef2308282b8285077534667f" src="https://github.com/vhive-serverless/vHive/assets/58351056/78cea3f8-b42f-4807-ad7a-10fea14a8eea">

This basically implies that the firecracker and gvisor cri runners need to be restart(You can also restart only one runner in that case)
But if the firecracker and gvisor cri test passed the `Setup vHive CRI test environment` step and failed in `Run vHive CRI tests` step, this typically is just sporadic failure and can be resolved by re-running the tests, just trigger the re-run button on github webpage is okay.

# Notice for Github PAT
Below are steps for generating github PAT:
1. On your personal github webpage, click `Developer settings` > `Personal access tokens` > `Tokens(classic)`, note that do not generate `Fine-grained tokens`
2. You can choose any expiration date. For PAT scopes, you can simply check the `repo`. 
3. Note that **NEVER** push your PAT to github or any other public spaces, it's unsafe to your github account and also, when github scans that your PAT is open for public access, the PAT is deprecated.