# Contributing to bottle-launch

Thank you for your interest in contributing to bottle-launch!

## Code Style

- Format code with `gofmt` before committing
- Run `go vet ./...` to catch common issues
- Use `golangci-lint run` for comprehensive linting (see `.golangci.yml`)

## Development Setup

1. Clone the repository
2. Ensure Go 1.22+ is installed
3. Run `make build` to verify setup
4. Run `make lint` to check code quality

## Pull Request Process

1. Fork the repository and create a feature branch
2. Make your changes with clear, focused commits
3. Ensure `make lint` passes without errors
4. Update documentation if adding new features
5. Submit a PR with a clear description of the changes

## Reporting Issues

When reporting bugs, please include:

- Go version (`go version`)
- Linux distribution and version
- Steps to reproduce the issue
- Expected vs actual behavior
- Any relevant error messages

## Security

If you discover a security vulnerability, please report it privately rather than opening a public issue.
