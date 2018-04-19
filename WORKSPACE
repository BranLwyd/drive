http_archive(
    name = "io_bazel_rules_go",
    sha256 = "feba3278c13cde8d67e341a837f69a029f698d7a27ddbb2a202be7a10b22142a",
    url = "https://github.com/bazelbuild/rules_go/releases/download/0.10.3/rules_go-0.10.3.tar.gz",
)

load("@io_bazel_rules_go//go:def.bzl", "go_rules_dependencies", "go_register_toolchains", "go_repository")

go_rules_dependencies()

go_register_toolchains()

go_repository(
    name = "org_golang_google_api",
    commit = "fca24fcb41126b846105a93fb9e30f416bdd55ce",
    importpath = "google.golang.org/api",
)

go_repository(
    name = "com_golang_x_oauth2",
    commit = "921ae394b9430ed4fb549668d7b087601bd60a81",
    importpath = "golang.org/x/oauth2",
)

go_repository(
    name = "com_golang_x_sync",
    commit = "1d60e4601c6fd243af51cc01ddf169918a5407ca",
    importpath = "golang.org/x/sync",
)

go_repository(
    name = "com_golang_x_time",
    commit = "fbb02b2291d28baffd63558aa44b4b56f178d650",
    importpath = "golang.org/x/time",
)

go_repository(
    # https://github.com/GoogleCloudPlatform/google-cloud-go
    name = "com_google_cloud_go",
    commit = "ce4a6d38fb6a22f17ff6bfccb76183cd85d609af",
    importpath = "cloud.google.com/go",
)
