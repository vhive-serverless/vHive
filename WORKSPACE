load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "io_bazel_rules_go",
    sha256 = "db2b2d35293f405430f553bc7a865a8749a8ef60c30287e90d2b278c32771afe",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/rules_go/releases/download/v0.22.3/rules_go-v0.22.3.tar.gz",
        "https://github.com/bazelbuild/rules_go/releases/download/v0.22.3/rules_go-v0.22.3.tar.gz",
    ],
)

load("@io_bazel_rules_go//go:deps.bzl", "go_rules_dependencies", "go_register_toolchains")

go_rules_dependencies()

go_register_toolchains()

http_archive(
    name = "bazel_gazelle",
    urls = [
        "https://storage.googleapis.com/bazel-mirror/github.com/bazelbuild/bazel-gazelle/releases/download/v0.20.0/bazel-gazelle-v0.20.0.tar.gz",
        "https://github.com/bazelbuild/bazel-gazelle/releases/download/v0.20.0/bazel-gazelle-v0.20.0.tar.gz",
    ],
    sha256 = "d8c45ee70ec39a57e7a05e5027c32b1576cc7f16d9dd37135b0eddde45cf1b10",
)

load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies", "go_repository")

gazelle_dependencies()

load("@bazel_tools//tools/build_defs/repo:git.bzl", "git_repository")

git_repository(
    name = "com_google_protobuf",
    commit = "09745575a923640154bcf307fba8aedff47f240a",
    remote = "https://github.com/protocolbuffers/protobuf",
    shallow_since = "1558721209 -0700",
)

load("@com_google_protobuf//:protobuf_deps.bzl", "protobuf_deps")

protobuf_deps()

go_repository(
    name = "com_github_pkg_errors",
    importpath = "github.com/pkg/errors",
    sum = "h1:FEBLx1zS214owpjy7qsBeixbURkuhQAwrK5UwLGTwt4=",
    version = "v0.9.1",
)

go_repository(
  name = "org_golang_google_grpc",
  importpath = "google.golang.org/grpc",
  sum = "h1:C1QC6KzgSiLyBabDi87BbjaGreoRgGUF5nOyvfrAZ1k=",
  version = "v1.28.1",
)

go_repository(
    name = "org_golang_x_net",
    importpath = "golang.org/x/net",
    sum = "h1:oWX7TPOiFAMXLq8o0ikBYfCJVlRHBcsciT5bXOrH628=",
    version = "v0.0.0-20190311183353-d8887717615a",
)

go_repository(
    name = "org_golang_x_text",
    importpath = "golang.org/x/text",
    sum = "h1:g61tztE5qeGQ89tm6NTjjM9VPIm088od1l6aSorWRWg=",
    version = "v0.3.0",
)

go_repository(
  name = "com_github_containerd_containerd",
  importpath = "github.com/containerd/containerd",
  sum = "h1:LoIzb5y9x5l8VKAlyrbusNPXqBY0+kviRloxFUMFwKc=",
  version = "v1.3.3",
)

go_repository(
  name = "com_github_containerd_containerd_ttrpc",
  importpath = "github.com/containerd/ttrpc",
#  sum = "h1:LoIzb5y9x5l8VKAlyrbusNPXqBY0+kviRloxFUMFwKc=",
#  version = "v1.3.3",
)

go_repository(
  name = "com_github_containerd_continuity",
  importpath = "github.com/containerd/continuity",
#  sum = "h1:LoIzb5y9x5l8VKAlyrbusNPXqBY0+kviRloxFUMFwKc=",
#  version = "v1.3.3",
)

go_repository(
  name = "com_github_containerd_fifo",
  importpath = "github.com/containerd/fifo",
#  sum = "h1:LoIzb5y9x5l8VKAlyrbusNPXqBY0+kviRloxFUMFwKc=",
#  version = "v1.3.3",
)

go_repository(
  name = "com_github_opencontainers_runtime_spec",
  importpath = "github.com/opencontainers/runtime-spec",
#  sum = "h1:LoIzb5y9x5l8VKAlyrbusNPXqBY0+kviRloxFUMFwKc=",
#  version = "v1.3.3",
)

go_repository(
  name = "com_github_opencontainers_image_spec",
  importpath = "github.com/opencontainers/image-spec",
#  sum = "h1:loizb5y9x5l8vkalyrbusnpxqby0+kvirloxfumfwkc=",
#  version = "v1.3.3",
)

go_repository(
  name = "com_github_opencontainers_go_digest",
  importpath = "github.com/opencontainers/go-digest",
#  sum = "h1:LoIzb5y9x5l8VKAlyrbusNPXqBY0+kviRloxFUMFwKc=",
#  version = "v1.3.3",
)

go_repository(
  name = "com_github_opencontainers_runc",
  importpath = "github.com/opencontainers/runc",
#  sum = "h1:LoIzb5y9x5l8VKAlyrbusNPXqBY0+kviRloxFUMFwKc=",
#  version = "v1.3.3",
)

go_repository(
  name = "com_github_syndtr_gocapability",
  importpath = "github.com/syndtr/gocapability",
#  sum = "h1:LoIzb5y9x5l8VKAlyrbusNPXqBY0+kviRloxFUMFwKc=",
#  version = "v1.3.3",
)

go_repository(
  name = "com_github_firecracker_microvm_firecracker_containerd",
  importpath = "github.com/firecracker-microvm/firecracker-containerd",
  sum = "h1:tjG0LFq22MnVjPFYcn/OjPUSjdckz0mI2E02Zh9z1AI=",
  version = "v0.0.0-20200324214552-7383119704ec",
)

go_repository(
  name = "com_github_davecgh_go_spew",
  importpath = "github.com/davecgh/go-spew",
  sum = "h1:vj9j/u1bqnvCEfJOwUhtlOARqs3+rkHYY13jYWTU97c=",
  version = "v1.1.1",
)
