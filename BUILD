load("@io_bazel_rules_go//go:def.bzl", "go_prefix", "go_binary")

go_prefix("github.com/BranLwyd/dsync")

go_binary(
    name = "dpush",
    srcs = ["dpush.go"],
    pure = "on",
    deps = [
        "@com_golang_x_oauth2//:go_default_library",
        "@com_golang_x_oauth2//google:go_default_library",
        "@org_golang_google_api//drive/v3:go_default_library",
    ],
)

go_binary(
    name = "dsync",
    srcs = ["dsync.go"],
    pure = "on",
    deps = [
        "@com_golang_x_oauth2//:go_default_library",
        "@com_golang_x_oauth2//google:go_default_library",
        "@com_golang_x_sync//errgroup:go_default_library",
        "@com_golang_x_time//rate:go_default_library",
        "@org_golang_google_api//drive/v3:go_default_library",
    ],
)
