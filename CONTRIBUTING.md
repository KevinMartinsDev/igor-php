# Contributing to Igor PHP 🧟‍♂️⚡

Thank you for your interest in contributing to **Igor PHP**! We welcome and appreciate all contributions, whether it's fixing bugs, adding new rules, improving documentation, or suggesting features.

To ensure a smooth collaboration, please read and follow these contribution guidelines.

---

## Table of Contents
1. [Reporting Issues](#reporting-issues)
2. [Development Setup](#development-setup)
3. [Project Architecture](#project-architecture)
4. [Testing & Quality Checks](#testing--quality-checks)
5. [Commit Guidelines](#commit-guidelines)
6. [Submitting a Pull Request](#submitting-a-pull-request)

---

## Reporting Issues

Before opening a new issue, please search the existing issues to see if it has already been reported.

- **Bug Reports**: If you find a bug, please use our **Bug Report template**. Be sure to include a minimal, reproducible PHP example that triggers the issue and state the expected vs. actual behavior.
- **Feature Requests**: If you have an idea for an enhancement or a new analysis rule, please use our **Feature Request template** to explain the use case and show target code snippets.

---

## Development Setup

Igor PHP is a hybrid tool: the static analysis engine is written in **Go**, and the Symfony configuration discovery/deep auditing is written in **PHP**.

### Prerequisites
- **Go**: 1.18 or higher.
- **PHP**: 8.1 or higher (required for deep audit and reflection).
- **Composer**: To manage PHP dependencies.

### Local Setup
1. Fork the repository on GitHub and clone it locally:
   ```bash
   git clone https://github.com/YOUR_USERNAME/igor-php.git
   cd igor-php
   ```

*Note: The PHP bundle has no third-party vendor dependencies, so no `composer install` is necessary to develop or run the tool!*

---

## Project Architecture

Understanding where everything lives will help you make targeted changes:

- `cmd/igor/`: The entry point for the Go command-line tool.
- `internal/analyzer/`: The core static analysis parser and visitor logic. This is where AST traversing and leak detection rules are defined.
- `internal/auditor/`: Logic for invoking the PHP bridge and auditing services.
- `pkg/reporter/`: Formatter for reports (console, JSON, LLM-friendly review).
- `src/php/`: The Symfony Bundle (`IgorPhpBundle`) that compiles container definitions.
- `test/fixtures/`: PHP code fixtures used in integration and visitor tests.
- `examples/demo-leak/`: The "Igor Leak Lab", a Docker-based Symfony playground used to demonstrate state pollution and memory leaks in FrankenPHP.

---

## Testing & Quality Checks

We use a standard `Makefile` to simplify local development, testing, and linting.

### 1. Build the Binary
To compile the Go executable:
```bash
make build
```
This builds the `igor` binary under `bin/igor`.

### 2. Run Tests
We have a comprehensive test suite including Go unit tests, AST visitor tests, and integration tests using real PHP fixtures.
```bash
make test
```
*Note: Ensure PHP is available on your path as integration tests call PHP scripts to test reflection and bundle discovery.*

### 3. Linting
We use `golangci-lint` to maintain Go code quality:
```bash
make lint
```

### 4. Full CI Validation
Before committing or pushing, run the entire local CI suite to ensure everything is correct:
```bash
make ci
```
This command runs `build`, `test`, and `lint` in sequence.

---

## Commit Guidelines

We follow the [Conventional Commits](https://www.conventionalcommits.org/) specification for clear and structured history.

Format: `<type>(<scope>): <description>`

Common types:
- `feat`: A new analysis rule or CLI feature.
- `fix`: A bug fix (e.g. false positive resolution, parsing error fix).
- `docs`: Documentation updates.
- `test`: Adding or correcting tests.
- `chore`: Maintenance tasks, dependencies updates.

Example:
```bash
feat(analyzer): detect local static variable assignment
fix(reporter): handle empty lines in CLI output formatting
```

---

## Submitting a Pull Request

1. Create a descriptive branch from `main`:
   ```bash
   git checkout -b feat/my-awesome-feature
   ```
2. Implement your changes and add corresponding tests:
   - Add a new PHP fixture under `test/fixtures/` and update the visitor or integration tests.
   - **If you are adding a new analysis rule**, ideally update our "Igor Leak Lab" laboratory under `examples/demo-leak/` (including services, the controller, and instructions) to demonstrate the issue in action.
3. Run `make ci` and fix any test failures or linter warnings.
4. Push your branch to your fork:
   ```bash
   git push -u origin HEAD
   ```
5. Open a Pull Request against the upstream repository.
6. Fill in the **Pull Request Template** with the required details.

Thank you again for helping make Igor PHP safer and faster! 🧟‍♂️⚡
