# Changelog

## [Unreleased]

### Added
- Support for MinIO S3 storage (non-replicated, non-distributed).
- MicroVMs can now access the internet by default using custom host interface. (default route interface is used if no argument is provided)
- Knative serving now can be tested separately from vHive. More info [here](https://github.com/ease-lab/vhive/wiki/Debugging-Knative-functions).

### Changed

### Fixed


## v1.1

### Added

- Created a CI pipeline that runs CRI integration tests inside containers using [kind](https://kind.sigs.k8s.io/).
- Added a developer image that runs a vHive k8s cluster in Docker, simplifying vHive development and debugging.
- Extended the developers guide on the modes of operation, performance analysis and vhive development environment inside containers.
- Added a slide deck of Dmitrii's talk at Amazon.
- Added a commit linter and a spell checker for `*.md` files.

### Changed

- Use replace pragmas instead of Go modules.
- Bump Go version to 1.15 in CI.
- Deprecated Travis CI.

### Fixed

- Fixed the vHive cluster setup issue for clusters with >2 nodes [issue](https://github.com/ease-lab/vhive/issues/94). 

