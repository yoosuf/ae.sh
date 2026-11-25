# Dependency-free Go demo app

A tiny module used by AEF examples and offline workflow demos. It has no third-party
dependencies, no network access, and its own `go.mod`, so it can be tested
independently of the repository root module.

```sh
cd examples/demo-app
go test ./...
go run .
```

`go test ./...` runs the table-driven test for `Greet`; `go run .` prints
`hello, world`.
