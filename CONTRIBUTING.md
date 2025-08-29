# Contributing

Thank you for considering a contribution to **mariadb2tidb**.

## Guidelines

- Format code with `make fmt` (gofmt -s).
- Lint locally with `golangci-lint run` (see `.golangci.yml`).
- Add tests for new features or bug fixes.
- Run `make test` before submitting.
- Update [CHANGELOG.md](CHANGELOG.md) with a summary of your change.

## Development Workflow

1. Fork the repository and create a branch.
2. Implement your change.
3. Run `make fmt vet test`.
4. Submit a pull request.
