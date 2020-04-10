load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")
load("@bazel_gazelle//:def.bzl", "gazelle")

# gazelle:prefix github.com/ustiugov/fccd-orchestrator
# gazelle:proto package
gazelle(name = "gazelle")

go_library(
    name = "go_default_library",
    srcs = ["fccd-orchestrator.go"],
    importpath = "github.com/ustiugov/fccd-orchestrator",
    visibility = ["//visibility:private"],
    deps = [
        "//ctrIface:go_default_library",
        "//helloworld:go_default_library",
        "//proto:proto_go_proto",
        "@com_github_pkg_errors//:go_default_library",
        "@org_golang_google_grpc//:go_default_library",
    ],
)

go_binary(
    name = "fccd-orchestrator",
    embed = [":go_default_library"],
    visibility = ["//visibility:public"],
)
