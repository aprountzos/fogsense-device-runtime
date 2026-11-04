# Contributing to FogSense Device Runtime

Thank you for your interest in contributing to **FogSense Device Runtime**! We welcome contributions of all kinds, including bug reports, feature requests, documentation improvements, and code changes.

Please take a moment to review this document before submitting your contribution.

---

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [How Can I Contribute?](#how-can-i-contribute)
  - [Reporting Bugs](#reporting-bugs)
  - [Suggesting Enhancements](#suggesting-enhancements)
  - [Improving Documentation](#improving-documentation)
  - [Contributing Code](#contributing-code)
- [Development Setup](#development-setup)
  - [Prerequisites](#prerequisites)
  - [Clone & Build](#clone--build)
  - [Running the Runtime Locally](#running-the-runtime-locally)
  - [Documentation Website](#documentation-website)
- [Testing & Quality Checks](#testing--quality-checks)
  - [Running Tests](#running-tests)
  - [Testing with Example Specs](#testing-with-example-specs)
- [Code Style & Best Practices](#code-style--best-practices)
- [Pull Request Process](#pull-request-process)

---

## Code of Conduct

This project and everyone participating in it is governed by the [Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code. Please report unacceptable behavior following the instructions in that document.

---

## How Can I Contribute?

### Reporting Bugs

Before creating a bug report:
1. Check the [issue tracker](https://github.com/aprountzos/fogsense-device-runtime/issues) to make sure the issue hasn't already been reported.
2. Verify that the bug reproduces with the latest version of the `main` branch.

When filing a bug, use the [Bug Report template](.github/ISSUE_TEMPLATE/bug_report.yml) and include:
- A clear, descriptive title.
- Steps to reproduce the problem.
- Expected behavior vs. actual behavior.
- Relevant device spec files (e.g. from `examples/` or a minimal YAML snippet).
- Console logs or error outputs.
- Your Go version, operating system, and whether you are running natively or inside Docker.

### Suggesting Enhancements

If you have an idea to improve the runtime, contract specification, or tooling:
1. Search the [existing issues](https://github.com/aprountzos/fogsense-device-runtime/issues) to see if a similar feature has been proposed.
2. Open a feature request using the [Feature Request template](.github/ISSUE_TEMPLATE/feature_request.yml).
3. Clearly explain the motivation, use case, and proposed design. If your proposal changes the device specification schema, explain backwards compatibility implications.

### Improving Documentation

Documentation lives in two places:
- The `README.md` at the project root for quick reference.
- The full Docusaurus website located in `docs/`, which is published to GitHub Pages.

Fixes for typos, clearer explanations, and new tutorials or examples are always welcome!

---

## Development Setup

### Prerequisites

- **Go**: Version 1.22 or newer (matches our CI matrix: 1.22, 1.23, 1.24).
- **Git**
- **Docker & Docker Compose** (optional, for containerized testing).
- **Node.js** v18+ & **npm** (optional, for working on the documentation site in `docs/`).

### Clone & Build

```bash
git clone https://github.com/aprountzos/fogsense-device-runtime.git
cd fogsense-device-runtime

# Download dependencies
go mod download

# Build the runtime
go build -v ./...
```

### Running the Runtime Locally

You can test the runtime alongside the bundled mock gateway:

```bash
# Terminal 1: Start mock gateway
go run ./cmd/mock-gateway --port 8081 --device-token secret --control-token control --check

# Terminal 2: Run device runtime with an example spec
# On Linux/macOS:
DEVICE_ID=dev-1 GATEWAY_URL=http://127.0.0.1:8081 DEVICE_TOKEN=secret CONTROL_TOKEN=control \
  ADVERTISE_URL=http://127.0.0.1:8080 DEVICE_SPEC_FILE=examples/thermostat.yaml go run .

# On Windows PowerShell:
$env:DEVICE_ID="dev-1"; $env:GATEWAY_URL="http://127.0.0.1:8081"; $env:DEVICE_TOKEN="secret"
$env:CONTROL_TOKEN="control"; $env:ADVERTISE_URL="http://127.0.0.1:8080"
$env:DEVICE_SPEC_FILE="examples/thermostat.yaml"
go run .
```

### Documentation Website

To preview or build the Docusaurus documentation site locally:

```bash
cd docs
npm install
npm start        # Launches live-reload server at http://localhost:3000
npm run build    # Validates links and produces production build
```

> **Note**: Our CI workflow fails if `npm run build` detects any broken links.

---

## Testing & Quality Checks

### Running Tests

Before submitting a pull request, run all unit and integration tests across packages:

```bash
go test -v ./...
```

To run tests with race detection:

```bash
go test -race ./...
```

### Testing with Example Specs

Validate that all specs in `examples/` load and execute without errors:

```bash
# Verify formatting and loading of each YAML spec
go test -v ./pkg/...
```

---

## Code Style & Best Practices

- **Formatting**: Always format your Go code with `gofmt` before committing:
  ```bash
  gofmt -s -w .
  ```
- **Error Handling**: Follow standard Go error idioms (`if err != nil`). Wrap errors with context where appropriate.
- **Spec Compatibility**: Contract changes (new spec fields, reason codes, heartbeat behaviors) must remain additive and backward-compatible whenever possible.
- **Minimal Dependencies**: Keep external dependencies minimal to preserve the lightweight nature of the device runtime binary.

---

## Pull Request Process

1. **Fork the repository** and create your branch from `main`:
   ```bash
   git checkout -b feature/my-new-feature
   ```
2. **Make your changes**: Write clean code and accompanying tests.
3. **Verify tests and formatting**:
   ```bash
   gofmt -s -w .
   go test -v ./...
   ```
   If you modified documentation, ensure `cd docs && npm run build` completes with zero broken links.
4. **Commit your changes**: Write descriptive, concise commit messages.
5. **Push to your fork** and submit a Pull Request to `aprountzos/fogsense-device-runtime` against the `main` branch.
6. **Fill out the PR template**: Reference any related issues (e.g., `Closes #12`) and describe what has changed.
7. **Address review feedback**: Maintainers or reviewers may suggest changes. Push new commits to your branch to update the PR.

