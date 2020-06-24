load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "io_bazel_rules_go",
    sha256 = "a8d6b1b354d371a646d2f7927319974e0f9e52f73a2452d2b3877118169eb6bb",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/rules_go/releases/download/v0.23.3/rules_go-v0.23.3.tar.gz",
        "https://github.com/bazelbuild/rules_go/releases/download/v0.23.3/rules_go-v0.23.3.tar.gz",
    ],
)

http_archive(
    name = "bazel_gazelle",
    sha256 = "cdb02a887a7187ea4d5a27452311a75ed8637379a1287d8eeb952138ea485f7d",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/bazel-gazelle/releases/download/v0.21.1/bazel-gazelle-v0.21.1.tar.gz",
        "https://github.com/bazelbuild/bazel-gazelle/releases/download/v0.21.1/bazel-gazelle-v0.21.1.tar.gz",
    ],
)

load("@io_bazel_rules_go//go:deps.bzl", "go_rules_dependencies", "go_register_toolchains")

go_rules_dependencies()

go_register_toolchains()

load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies", "go_repository")

gazelle_dependencies()


go_repository(
    name = "com_github_golang_groupcache",
    commit = "8c9f03a8e57eb486e42badaed3fb287da51807ba",
    importpath = "github.com/golang/groupcache",
)

go_repository(
    name = "com_github_googleapis_gax_go_v2",
    commit = "be11bb253a768098254dc71e95d1a81ced778de3",
    importpath = "github.com/googleapis/gax-go",
)

go_repository(
    name = "com_github_thomaso-mirodin_intmath",
    commit = "5dc6d854e46e8db72326367254b8de5d2c5f2f4f",
    importpath = "github.com/thomaso-mirodin/intmath",
)

go_repository(
    name = "com_golang_x_net",
    commit = "627f9648deb96c27737b83199d44bb5c1010cbcf",
    importpath = "golang.org/x/net",
)

go_repository(
    name = "com_golang_x_oauth2",
    commit = "bf48bf16ab8d622ce64ec6ce98d2c98f916b6303",
    importpath = "golang.org/x/oauth2",
)

go_repository(
    name = "com_golang_x_sync",
    commit = "43a5402ce75a95522677f77c619865d66b8c57ab",
    importpath = "golang.org/x/sync",
)

go_repository(
    name = "com_golang_x_text",
    commit = "23ae387dee1f90d29a23c0e87ee0b46038fbed0e",
    importpath = "golang.org/x/text",
)

go_repository(
    name = "com_golang_x_time",
    commit = "89c76fbcd5d1cd4969e5d2fe19d48b19d5ad94a0",
    importpath = "golang.org/x/time",
)

go_repository(
    # https://github.com/GoogleCloudPlatform/google-cloud-go
    name = "com_google_cloud_go",
    commit = "f1a22b3f1e6b4740b068f420be3b704b31cb5ab4",
    importpath = "cloud.google.com/go",
)

go_repository(
    name = "com_google_protobuf",
    commit = "2b7b7f7f72e3617191972fbafb298cf7ec31e95e",
    importpath = "github.com/protocolbuffers/protobuf",
)

go_repository(
    name = "org_golang_google_api",
    commit = "9d4f8465cab9f7314bfa2d615f33d7c675c1f43d",
    importpath = "google.golang.org/api",
)

go_repository(
    name = "org_golang_google_grpc",
    commit = "38aafd89f814f347db56a52efd055961651078ad",
    importpath = "google.golang.org/grpc",
)

go_repository(
    name = "io_opencensus_go",
    commit = "5fa069b99bc903d713add0295c7e0a55d34ae573",
    importpath = "go.opencensus.io",
)

load("@com_google_protobuf//:protobuf_deps.bzl", "protobuf_deps")

protobuf_deps()
