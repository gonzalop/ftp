# Contributing to FTP Client Library

Thank you for your interest in contributing! This library aims to be a production-ready, RFC-compliant FTP client for Go.

## How to Contribute

### Reporting Issues

- Check if the issue already exists in the [issue tracker](https://github.com/gonzalop/ftp/issues)
- Provide a clear description of the problem
- Include code examples that reproduce the issue
- Specify your Go version and operating system

### Submitting Pull Requests

1. **Fork the repository** and create your branch from `main`
2. **Write tests** for any new functionality
3. **Ensure all tests pass**: `go test -v ./...`
4. **Follow Go conventions**: Run `go fmt` and `go vet`
5. **Update documentation** if you're changing APIs
6. **Reference any related issues** in your PR description

### Development Setup

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/ftp.git
cd ftp

# Run tests
go test -v ./...

# Run tests with coverage
go test -v -race -coverprofile=coverage.txt ./...

# Build
go build -v ./...
```

### Code Style

- Follow standard Go formatting (`go fmt`)
- Write clear, descriptive commit messages
- Add godoc comments for exported functions
- Keep functions focused and testable

### Testing

- Write unit tests for new features
- Ensure tests are deterministic
- Mock external dependencies where appropriate
- Aim for high test coverage

### RFC Compliance

This library aims for RFC 5797 compliance. When adding new commands:
- Reference the relevant RFC
- Update `RFC5797-compliance.md`
- Add examples to `examples_test.go`

## Questions?

Feel free to open an issue for questions or discussions!

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
