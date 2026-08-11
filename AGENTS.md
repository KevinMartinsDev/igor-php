# igor-php 🧟‍♂️⚡

`igor-php` is an ultra-fast static linter written in **Go** that prepares **Symfony** applications for the persistent memory model of **FrankenPHP Workers** by checking shared services and detecting state pollution or memory leaks.

---

## 📖 Reference Documents & Guidelines

As an AI agent, you **must** read, understand, and strictly adhere to the standards and processes defined in these key documentation files:
- **`README.md`**: Contains the rules catalog, configuration options (`igor.json`), and comprehensive feature specifications.
- **`CONTRIBUTING.md`**: Outlines the developer prerequisites, repository architecture, test suite execution steps, and local setup guidelines.
- **`Makefile`**: Defines all standard commands for compiling (`make build`), running tests (`make test`), linting (`make lint`), and executing the full local validation pipeline (`make ci`).
- **`.github/pull_request_template.md`**: The mandatory schema and checklist that must be used when drafting any Pull Request descriptions.
- **`.github/ISSUE_TEMPLATE/`**: The structured templates for raising bug reports or proposing new feature rules.

---

## 🛠 Stack & Architecture

- **Go**: 1.18+ (The main static analysis engine and CLI tool).
- **PHP**: 8.1+ (Used for the container discovery bundle and deep audit/reflection to resolve service classes).
- **Framework Integration**: Symfony 6.4/7.0/8.0+ support via a lightweight custom PHP bundle (`IgorPhpBundle`), which runs with zero third-party dependencies.

---

## 📂 Key Components & File Paths

- **CLI Entry point**: `cmd/igor/main.go`
- **AST Visitor & Detection Rules**: `internal/analyzer/visitor.go` (traverses PHP AST nodes to flag mutations, static state, execution terminators, superglobals, and closure leaks).
- **Symfony Service Finder**: `internal/auditor/symfony.go` and `internal/auditor/find_class_files.php` (calls Symfony commands to dump container configuration).
- **Symfony Discovery Bundle**: `src/php/IgorPhpBundle.php` and `src/php/DependencyInjection/Compiler/IgorDiscoveryPass.php`.
- **Local Lab Playground**: `examples/demo-leak/` (the Docker-based Symfony playground for simulating leaks in persistent memory).

---

## 🧪 Testing Strategy

- **Go Test Suite**: Unit, visitor, and integration tests can be run using `make test` or `go test ./...` (or **`make docker-test`** inside Docker, which is **highly preferred** to guarantee a clean environment).
- **PHP Fixtures**: Static code fixtures used in integration and visitor tests are located in `test/fixtures/`.
- **Formatting & Linting**: Run `make lint` to execute `golangci-lint` (or **`make docker-lint`** inside Docker, which is **highly preferred**).
- **Full local CI loop**: `make ci` (or **`make docker-ci`** inside Docker, which is **highly preferred**) executes `build`, `test`, and `lint` in sequence.

---

## 💡 Guidelines for Agents

### 1. Adding a New Static Analysis Rule
- Define the node matching and analysis logic inside `internal/analyzer/visitor.go`.
- Create a new PHP file under `test/fixtures/` demonstrating the correct detection and ignore cases.
- Update `internal/analyzer/visitor_test.go` or `cmd/igor/integration_test.go` with your test assertions using the new fixture.
- **Ideally**, update the Docker laboratory under `examples/demo-leak/` (services, controller, and instructions) to showcase this new pattern and its impacts under persistent FrankenPHP worker runtime.

### 2. No External PHP Dependencies
- The Symfony Bundle under `src/php/` must remain dependency-free.
- Its `composer.json` should only require `"php": ">=8.1"`. Never add third-party PHP packages to `require`.

### 3. Local Verification
- **Always prioritize executing tests and CI loops inside Docker.** Always run **`make docker-ci`** (or `make docker-test` / `make docker-lint`) before suggesting or committing changes to ensure formatting, linting, and Go/PHP integration tests pass flawlessly in a consistent environment.
