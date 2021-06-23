# Contributing to vHive
This document gives a brief overview of how you can contribute to the vHive repository.

## Code Style
### Comments and Documentation
All vHive documentation can be found [here](https://pkg.go.dev/github.com/ease-lab/vhive). When contributing code please make sure do document it appropriately, as described in [this guide](https://blog.golang.org/godoc).

There is no need for excessive comments within the code itself and we prefer brevity where possible.

## Pull Requests
When contributing to the repository you should work in a separate branch and create a GitHub pull request for your branch. For all pull requests to vHive we require that you do the following:
- Sync your Repo
- Squash commits
- Rebase on Main
- Avoid Merges

### Syncing your GitHub Repos
When you are working on a fork of the vHive repository, keeping your fork in sync with the main repository keeps your workspace up-to-date and reduces the risk of merge conflicts. 

The most common way of syncing up your fork is with a remote that points to the upstream repository:
1. If you have not done so already, create a new remote for the upstream vHive repo:
	```bash
	git remote add upstream https://github.com/ease-lab/vhive.git
	```
	You can always check your existing remotes with `git remote -v`.
2. Fetch branches and commits from the upstream (vHive) repo:
	```bash
	git fetch upstream
	```
3. Switch to your local default branch (named `main` by default):
	```bash
	git checkout main
	```
4. Merge the upstream changes:
	```bash
	git merge upstream/main
	```

You can check out [the official GitHub docs](https://docs.github.com/en/github/collaborating-with-pull-requests/working-with-forks/syncing-a-fork) for more information on syncing forks.

### Squashing Commits
We prefer for every commit in the repo to encapsulate a single concrete and atomic change/addition. This means our commit history shows a clear and structured progression, and that it remains concise and readable. In your own branch you can clean up your commit history by squashing smaller commits together using `git rebase`:
1. Find the hash of the oldest commit which you want to squash up to. For example, if you made three commits in sequence (A, B, C, such that C is the latest commit) and you wanted to squash B and C then you would need to find the hash of A. You can find the hash of a commit on GitHub or by using the command:
	```bash
	git log
	```
2. Use the rebase command in interactive mode:
	```bash
	git rebase -i [your hash]
	```
3. For each commit which you would like to squash, replace "pick" with "s". Keep in mind that the "s" option keeps the commit but squashes it into the previous commit, i.e. the one above it. For example, consider the following:
	```
	pick 4f3d934 commit A
	s c24c160 commit B
	s f20ac90 commit C
	pick 7667d38 commit D
	```
	This would squash commits A, B, and C into a single commit, and then commit D would be left as a separate commit.
4. Update the commit messages as prompted.
5. Push your changes:
	```bash
	git push --force
	```

Apart from squashing commits, `git rebase -i` can also be used for rearranging the order of commits. If you are currently working on a commit and you already know that you will need to squash it with the previous commit at some point in the future, you can also use `git commit --amend` which automatically squashes with the last commit. 

### Rebasing on the Main Branch
By rebasing our branch on the head of the main branch we avoid merge pull requests, since all the changes on our branch will build forward from the main branch. By rebasing we move the starting point of our branch, for example if we start with a commit tree like this:
```
      A---B---C branch
     /
D---E---F---G master
```
Then we are interested in rebasing such that the tree ends up like this:
```
              A'--B'--C' branch
             /
D---E---F---G master
```
For more detail check out [the git rebase documentation](https://git-scm.com/docs/git-rebase).

To rebase on main simply checkout your branch and use the following:
```bash
git rebase main
```

### Avoiding Merge Commits
As mentioned throughout this document, merge commits should be avoided. By having a linear commit history the changes made to the project are more clean, readable, and structured, and avoiding merged pull requests also makes the PR easier to incorporate into the main branch. To avoid merge commits one should follow the guidelines described above, particularly taking care to rebase on the main branch and keeping your forks in sync.

## Recommendations
### Using Go modules with private Git repo
We might need to use private Go modules in a public repo. This tutorial enables us to inject our private Go module in a public repo without making our code public. This is a simple 2 step process. 
1. Bypass the default go proxy:
Set GOPRIVATE environment variable to comma separated list of github repo/account for which are private.
```bash
# Source https://medium.com/swlh/go-modules-with-private-git-repository-3940b6835727
go env -w GOPRIVATE=github.com/user1,github.com/user2/repo
```
2. Automating the private repo login during build
    1. Click [here](https://github.com/settings/tokens) to create a Github personal access token from [here](https://github.com/settings/tokens/new). The token should have enough permissions to read private codebase.
    2. Execute the following command to use ssh-keys authentication.
    ```bash
    # Source https://medium.com/swlh/go-modules-with-private-git-repository-3940b6835727
    # username is the owner of the repository
    # access_token is the token created above
    git config --global url."https://${username}:${access_token}@github.com".insteadOf "https://github.com/${username}"
    ```
