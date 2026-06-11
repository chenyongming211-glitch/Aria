# File Naming Conventions

This repository uses language-native file naming instead of one global rule for
every file type.

## General Rules

- Use lowercase path segments for ordinary directories.
- Use `kebab-case` for Markdown documents, deployment YAML, JSON dashboards, and
  long-lived planning documents.
- Use `snake_case` for Go, Rust, and protobuf source files.
- Avoid spaces, mixed separators, and case-only filename differences.
- Keep generated files aligned with their source filename.

## Preserved Conventional Names

The following names are intentionally not normalized:

- `README.md`, `CHANGELOG.md`, and `VERSION`
- `Dockerfile` and `Dockerfile.*`
- `Cargo.toml`, `Cargo.lock`, `go.mod`, `go.sum`, `package.json`, and
  `package-lock.json`

## Go, Rust, and Protobuf

- Go files use `snake_case.go`.
- Rust modules use `snake_case.rs`.
- Protobuf files use `snake_case.proto`.
- Generated protobuf files keep the same base name, for example:
  - `aria_agent.proto`
  - `aria_agent.pb.go`
  - `aria_agent_grpc.pb.go`

## Frontend

- Vue single-file components use `PascalCase.vue`.
- Component directories use lowercase names.
- Composables use the Vue convention `useXxx.js`.
- Unit test filenames may mirror the source module they test.

## Documentation

- Documentation under `docs/` uses lowercase `kebab-case.md`.
- Historical implementation notes under `.kiro/` also use lowercase
  `kebab-case.md`.
