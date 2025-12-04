This guide describes how to make changes to Knative Serving and use the changed version of Knative in vHive.

## Build Knative
1. Install [Knative requirements](https://github.com/knative/serving/blob/main/DEVELOPMENT.md#install-requirements):
   1. Install [`ko`](https://github.com/google/ko):
      ```sh
      wget -qO- https://github.com/google/ko/releases/download/v0.15.1/ko_0.15.1_Linux_x86_64.tar.gz | sudo tar -C /usr/bin/ -xz ko
      sudo chmod +x /usr/bin/ko
      ```
   2. Install `go` (1.25 or later).
   3. Install `git`; already installed on CloudLab.
4. Login to Docker Hub account that the Knative images will be pushed to.
   ```sh
   ko login index.docker.io -u <DOCKERHUB USERNAME> -p <DOCKERHUB PASSWORD>
   ```
   :warning: Note that you shouldn't use root permission in this step, otherwise, error will be thrown at step 5. If you find yourself using `sudo`, something may have gone wrong and you'd better reset your environment.
5. [Set up your environment](https://github.com/knative/serving/blob/main/DEVELOPMENT.md#set-up-your-environment) for building:
   ```sh
   cat << EOF >> ~/.bashrc
   export GOROOT=$(go env GOROOT)
   export GOPATH="$HOME/go"
   export PATH="${PATH}:${GOPATH}/bin"
   export KO_DOCKER_REPO='docker.io/<DOCKERHUB USERNAME>'
   EOF
   ```
   - `<DOCKER HUB USERNAME>` must be the same username that you used to login in the previous step.
6. Git clone your fork:
   ```sh
   git clone --branch=<YOUR PR BRANCH> https://github.com/vhive-serverless/serving
   cd serving
   ```
7. Generate new Knative YAMLs (by building the relevant Docker images and uploading them to Docker Hub too):
   ```sh
   ./hack/generate-yamls.sh . new-yamls.txt
   ```
8. Copy the generated Knative YAMLs to a well-known location:
   ```sh
   mkdir -p ~/new-yamls
   cp $(cat new-yamls.txt) ~/new-yamls
   ```
9. Copy the generated Knative YAMLs to `configs/knative_yamls/` in vhive repo your local machine if you have used a remote server---**execute the following on your local machine**:
   ```sh
   # Say, you are at the root of vhive-serverless/vhive repository...
   rsync -zr <HOSTNAME>:new-yamls/ configs/knative_yamls/ --progress=info2
   ```

   Files from `configs/knative_yamls/` will be used instead of stock one during setup.
10. Commit and push/merge your changes to/at both repos!

## Update knative version in vHive

In order to use the Firecracker in vHive, we need to patch queue-proxy image (like it is done [here](https://github.com/vhive-serverless/serving/pull/2)). The resulting image of queue-proxy should be pushed to [registry](https://github.com/vhive-serverless/serving/pkgs/container/queue-39be6f1d08a095bd076a71d288d295b6) and the url of the image should be updated in `configs/setup/knative.json`.

Other necessary change is to update the version in `configs/setup/knative.json`.