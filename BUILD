load("@io_bazel_rules_go//go:def.bzl", "go_prefix", "go_binary", "go_library")

go_prefix("github.com/BranLwyd/drive")

##
## Binaries
##
go_binary(
    name = "dpush",
    srcs = ["dpush.go"],
    pure = "on",
    deps = [
        ":cli",
        ":client",
        "@org_golang_google_api//drive/v3:go_default_library",
    ],
)

go_binary(
    name = "dsync",
    srcs = ["dsync.go"],
    pure = "on",
    deps = [
        ":cli",
        ":client",
        "@com_github_thomaso-mirodin_intmath//i64:go_default_library",
        "@com_golang_x_sync//errgroup:go_default_library",
        "@com_golang_x_time//rate:go_default_library",
        "@org_golang_google_api//drive/v3:go_default_library",
    ],
)

##
## Libraries
##
go_library(
    name = "cli",
    srcs = ["cli.go"],
)

go_library(
    name = "client",
    srcs = ["client.go"],
    deps = [
        "@com_golang_x_oauth2//:go_default_library",
        "@com_golang_x_oauth2//google:go_default_library",
        "@org_golang_google_api//drive/v3:go_default_library",
    ],
)
