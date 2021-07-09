# Self-Hosted Stock-Knative on KinD Runners

## Setup
1. Install Ansible on your **local** machine: \
   https://docs.ansible.com/ansible/latest/installation_guide/intro_installation.html#installing-ansible-on-specific-operating-systems
2. Start a new experiment on Cloudlab:
    - Profile: `small-lan`
    - OS Image: `UBUNTU 18.04`
    - Physical Node Type: `rs440` (others are probably fine too)
    - (Under "Advanced") Temp Filesystem Max Space: Selected
3. On your **local** machine, use Ansible to setup the remote host:
   ```bash
   ansible-playbook -u YOUR_SSH_USERNAME -i REMOTE_HOSTNAME, setup-host.yaml
   ```
4. On your **local** machine, use Ansible to create a stock-Knative runner on KinD on the remote host:
   ```bash
   GH_ACCESS_TOKEN=... ansible-playbook -u YOUR_SSH_USERNAME -i REMOTE_HOSTNAME, create-runner.yaml
   ```
   - `GH_ACCESS_TOKEN` environment variable shall be set to your [GitHub access token](https://docs.github.com/en/github/authenticating-to-github/keeping-your-account-and-data-secure/creating-a-personal-access-token).

You may call the fourth step as many times as you like. To create _N_=10 runners, execute the following on bash:
```bash
for run in {1..10}; do GH_ACCESS_TOKEN=... ansible-playbook -u YOUR_SSH_USERNAME -i REMOTE_HOSTNAME, create-runner.yaml; done
```

## Deleting All Runners
On your **local** machine, use Ansible to delete all runners:
```bash
ansible-playbook -u YOUR_SSH_USERNAME -i REMOTE_HOSTNAME, delete-runners.yaml
```

## Restarting the Host
1. On your **local** machine, use Ansible to delete all runners:
   ```bash
   ansible-playbook -u YOUR_SSH_USERNAME -i REMOTE_HOSTNAME, delete-runners.yaml
   ```
2. Restart the remote host:
   ```bash
   sudo shutdown --reboot now
   ```
3. On your **local** machine, use Ansible to recreate runners (_N_=10) on the remote host:
   ```bash
   for run in {1..10}; do GH_ACCESS_TOKEN=... ansible-playbook -u YOUR_SSH_USERNAME -i REMOTE_HOSTNAME, create-runner.yaml; done
   ```