load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "io_bazel_rules_go",
    sha256 = "b34cbe1a7514f5f5487c3bfee7340a4496713ddf4f119f7a225583d6cafd793a",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/rules_go/releases/download/v0.21.1/rules_go-v0.21.1.tar.gz",
        "https://github.com/bazelbuild/rules_go/releases/download/v0.21.1/rules_go-v0.21.1.tar.gz",
    ],
)

load("@io_bazel_rules_go//go:deps.bzl", "go_register_toolchains", "go_rules_dependencies")

go_rules_dependencies()

go_register_toolchains()

http_archive(
    name = "bazel_gazelle",
    sha256 = "86c6d481b3f7aedc1d60c1c211c6f76da282ae197c3b3160f54bd3a8f847896f",
    urls = [
        "https://storage.googleapis.com/bazel-mirror/github.com/bazelbuild/bazel-gazelle/releases/download/v0.19.1/bazel-gazelle-v0.19.1.tar.gz",
        "https://github.com/bazelbuild/bazel-gazelle/releases/download/v0.19.1/bazel-gazelle-v0.19.1.tar.gz",
    ],
)

load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies", "go_repository")

gazelle_dependencies()

go_repository(
    name = "com_github_golang_groupcache",
    commit = "8c9f03a8e57eb486e42badaed3fb287da51807ba",
    importpath = "github.com/golang/groupcache",
)

go_repository(
    name = "com_github_googleapis_gax_go_v2",
    commit = "b443e5a67ec8eeac76f5f384004931878cab24b3",
    importpath = "github.com/googleapis/gax-go",
)

go_repository(
    name = "com_github_thomaso-mirodin_intmath",
    commit = "5dc6d854e46e8db72326367254b8de5d2c5f2f4f",
    importpath = "github.com/thomaso-mirodin/intmath",
)

go_repository(
    name = "com_golang_x_net",
    commit = "16171245cfb220d5317888b716d69c1fb4e7992b",
    importpath = "golang.org/x/net",
)

go_repository(
    name = "com_golang_x_oauth2",
    commit = "bf48bf16ab8d622ce64ec6ce98d2c98f916b6303",
    importpath = "golang.org/x/oauth2",
)

go_repository(
    name = "com_golang_x_sync",
    commit = "cd5d95a43a6e21273425c7ae415d3df9ea832eeb",
    importpath = "golang.org/x/sync",
)

go_repository(
    name = "com_golang_x_text",
    commit = "929e72ca90deac4784bbe451caf10faa5b256ebe",
    importpath = "golang.org/x/text",
)

go_repository(
    name = "com_golang_x_time",
    commit = "555d28b269f0569763d25dbe1a237ae74c6bcc82",
    importpath = "golang.org/x/time",
)

go_repository(
    # https://github.com/GoogleCloudPlatform/google-cloud-go
    name = "com_google_cloud_go",
    commit = "1eb0d529b77512b248a3d0f239f83e1ef8fa1162",
    importpath = "cloud.google.com/go",
)

go_repository(
    name = "com_google_protobuf",
    commit = "4cf5bfee9546101d98754d23ff378ff718ba8438",
    importpath = "github.com/protocolbuffers/protobuf",
)

go_repository(
    name = "org_golang_google_api",
    commit = "a349051e77e3050972630baf393750b4f1555a16",
    importpath = "google.golang.org/api",
)

go_repository(
    name = "org_golang_google_grpc",
    commit = "1f6cca9665f91f74507163ea81453475a73dca97",
    importpath = "google.golang.org/grpc",
)

go_repository(
    name = "io_opencensus_go",
    commit = "d835ff86be02193d324330acdb7d65546b05f814",
    importpath = "go.opencensus.io",
)

load("@com_google_protobuf//:protobuf_deps.bzl", "protobuf_deps")

protobuf_deps()
