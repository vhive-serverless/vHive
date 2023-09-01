# Contributing to vHive

This document gives a brief overview of how you can contribute to the vHive repository.

## Code Style

### Comments and Documentation

All vHive documentation can be found [here](https://pkg.go.dev/github.com/vhive-serverless/vhive).
When contributing code please make sure do document it appropriately, as described in [this guide](https://blog.golang.org/godoc).

There is no need for excessive comments within the code itself and we prefer brevity where possible.

## Pull Requests

When contributing to the repository you should work in a separate branch and create a GitHub pull request for your branch.
For all pull requests to vHive we require that you do the following:

- Sync your Repo
- Squash commits
- Rebase on Main
- Avoid Merges

### Syncing your GitHub Repos

When you are working on a fork of the vHive repository, keeping your fork in sync
with the main repository keeps your workspace up-to-date and reduces the risk of merge conflicts.

The most common way of syncing up your fork is with a remote that points to the upstream repository:

1. If you have not done so already, create a new remote for the upstream vHive repo:
   ```bash
   git remote add upstream https://github.com/vhive-serverless/vhive.git
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

You can check out [the official GitHub docs](https://docs.github.com/en/github/collaborating-with-pull-requests/working-with-forks/syncing-a-fork)
for more information on syncing forks.

### Commit Sign-off

Maintaining a clear and traceable history of contributions is essential for the integrity
and accountability of our project. To achieve this, we require that all contributors sign off on their Git commits.
This process ensures that you, as a contributor, acknowledge and agree to the terms of our project's
licensing and contribution guidelines.

#### How to add a Sign-off

To add a sign-off to your commit message, you can use the `-s` or `--signoff` flag with the `git commit` command:

```bash
git commit -s -m "Your commit message"
```

Alternatively, you can manually add the sign-off line to your commit message, like this:

```bash
Your commit message

Signed-off-by: Your Name <your.email@example.com>
```

#### Consequences of Not Signing Off

Commits that do not include a valid sign-off will not be accepted into the main branch of the repository.
Failure to comply with this requirement may result in the rejection of your contributions.

### Squashing Commits

We prefer for every commit in the repo to encapsulate a single concrete and atomic change/addition.
This means our commit history shows a clear and structured progression, and that it remains concise and readable.
In your own branch you can clean up your commit history by squashing smaller commits together using `git rebase`:

1. Find the hash of the oldest commit which you want to squash up to.
   For example, if you made three commits in sequence (A, B, C, such that C is the latest commit)
   and you wanted to squash B and C then you would need to find the hash of A.
   You can find the hash of a commit on GitHub or by using the command:
   ```bash
   git log
   ```
2. Use the rebase command in interactive mode:
   ```bash
   git rebase -i [your hash]
   ```
3. For each commit which you would like to squash, replace "pick" with "s".
   Keep in mind that the "s" option keeps the commit but squashes it into the previous commit,
   i.e. the one above it. For example, consider the following:
   ```
   pick 4f3d934 commit A
   s c24c160 commit B
   s f20ac90 commit C
   pick 7667d38 commit D
   ```
   This would squash commits A, B, and C into a single commit,
   and then commit D would be left as a separate commit.
4. Update the commit messages as prompted.
5. Push your changes:
   ```bash
   git push --force
   ```

Apart from squashing commits, `git rebase -i` can also be used for rearranging the order of commits.
If you are currently working on a commit and you already know that you will need to squash it
with the previous commit at some point in the future, you can also use `git commit --amend`
which automatically squashes with the last commit.

### Rebasing on the Main Branch

Rebasing is a powerful technique in Git that allows you to integrate changes from one branch into another.
When we rebase our branch onto the main branch, we create a linear history and avoid merge commits.
This is particularly valuable for maintaining a clean and structured commit history.

#### Why rebase?

Consider a scenario where you have a branch (`feature`) that you started from the `main` branch,
and both have received new commits since your branch was created:

```
      A---B---C feature
     /
D---E---F---G master
```

When you rebase your branch onto the main branch, Git rewrites the commit history.
It moves the starting point of your branch to the tip of the main branch. Here's what it looks like:

```
              A'--B'--C' feature
             /
D---E---F---G master
```

As you can see, your branch's commits (`A`, `B`, `C`) now build forward from the latest `main` commit (`G`).
This makes the history linear and eliminates the need for merge commits.

#### A messy merge without rebasing

Without rebasing, when the developer creates a merge commit, the history can become cluttered:

```
        A---B---C feature
       /           \
D---E---F---G-------M main
```

In this scenario, a merge commit (`M`) was created to combine the changes from the `feature` branch
into the `main` branch.
This results in a branching history, making it harder to follow the chronological order of changes
and potentially introducing unnecessary complexity.

#### How to rebase on Main

To rebase on `main` simply checkout your branch and use the following:

```bash
git checkout your-branch
git rebase main
```

This command sequence switches to your branch and reapplies your changes on top of the latest main branch.
It's important to resolve any conflicts that may arise during the rebase process.

For more details and options, refer to [the git rebase documentation](https://git-scm.com/docs/git-rebase).

### Maintaining a Clean Main Branch & Avoiding Merge Commits

Maintaining a clean and linear history on your main branch is essential for several reasons.

1. It simplifies the process of syncing the forked repository (e.g. vHive) with the upstream repository
   (e.g. AWS Firecracker).
   This is crucial because upstream repositories frequently receive updates and improvements from various contributors.
   To keep the forked repository up-to-date with these changes, we need to synchronize with the upstream repository.
   A clean main branch simplifies this process.
2. A clean main branch minimizes the likelihood of merge conflicts when you synchronize your fork with
   the upstream repository.
   It allows Git to efficiently compare changes between your main branch and the upstream repository's main branch.
3. A clean main branch enhances collaboration by making it easier for you and your collaborators to review and
   understand the project's history.
   It facilitates the review process, especially when you submit pull requests or collaborate with other developers.

To avoid merge commits one should follow the guidelines described above,
particularly taking care to rebase on the main branch and keeping your forks in sync.

## Recommendations

### Using Go modules with private Git repo

We might need to use private Go modules in a public repo. This tutorial enables us to inject
our private Go module in a public repo without making our code public. This is a simple 2 step process.

1. Bypass the default go proxy:
   Set GOPRIVATE environment variable to comma separated list of github repo/account for which are private.

```bash
# Source https://medium.com/swlh/go-modules-with-private-git-repository-3940b6835727
go env -w GOPRIVATE=github.com/user1,github.com/user2/repo
```

2. Automating the private repo login during build
   1. Click [here](https://github.com/settings/tokens) to create a Github personal access token from
      [here](https://github.com/settings/tokens/new). The token should have enough permissions to read private codebase.
   2. Execute the following command to use ssh-keys authentication.
   ```bash
   # Source https://medium.com/swlh/go-modules-with-private-git-repository-3940b6835727
   # username is the owner of the repository
   # access_token is the token created above
   git config --global url."https://${username}:${access_token}@github.com".insteadOf "https://github.com/${username}"
   ```
