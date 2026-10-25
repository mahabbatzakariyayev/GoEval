# Publishing GoEval

The module is `github.com/mahabbatzakariyayev/GoEval`, with root package `goeval`.
Publish this repository at that exact path. The installation command in README
becomes usable after the source is pushed to GitHub.

## Prepare a version

1. Review [coverage](coverage.md), README examples, LICENSE, and NOTICE.
2. Set `Version` in `internal/core/version.go` to the release version without the
   `v` prefix, for example `0.1.0` or `0.1.0-rc.1`.
3. Run `make check`, `make lint`, and `make example` on a modern Go toolchain.
   Confirm the GitHub CI matrix also passes before tagging.
4. Commit and push the reviewed changes to `main` through your normal workflow.

The checkout's origin is the destination GitHub repository and its main history
is the base for the local changes. No source publication or release is performed
by preparing these files.

## Tag and verify

For an initial stable development release:

```sh
git tag -a v0.1.0 -m "GoEval v0.1.0"
git push origin v0.1.0
```

Use the matching tag for a release candidate, such as `v0.1.0-rc.1`. Do not reuse
published tags. GitHub's generated source archives contain the Go package,
LICENSE, and NOTICE; a library release does not require platform binaries.

In a separate Go module, check the published version:

```sh
go get github.com/mahabbatzakariyayev/GoEval@v0.1.0
go list -m github.com/mahabbatzakariyayev/GoEval
```

Create GitHub release notes describing the supported features, API changes, and
known limits. Package documentation is available through the Go package index
after it discovers the published module. For a future v2 release, follow Go's
major-version module path rules.
