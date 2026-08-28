# Testing & Linting

Reference file for the bash skill. Read this when testing shell scripts or setting up linting.

## ShellCheck

ShellCheck is a static analysis tool for shell scripts. It catches real bugs: unquoted variables, useless uses of `cat`, unreachable code, incorrect redirections, and more.

### Installation

```bash
# macOS
brew install shellcheck

# Debian/Ubuntu
apt-get install shellcheck

# Via Docker
docker run --rm -v "$PWD:/mnt" koalaman/shellcheck:stable /mnt/script.sh
```

### Usage

```bash
# Check a single script
shellcheck script.sh

# Check all scripts in a directory
shellcheck scripts/*.sh

# Specify shell dialect
shellcheck --shell=bash script.sh

# Output as JSON (for CI parsing)
shellcheck --format=json script.sh

# Exclude specific warnings
shellcheck --exclude=SC2034,SC2086 script.sh
```

### Common Warnings

| Code | Issue | Fix |
|------|-------|-----|
| SC2086 | Double quote to prevent globbing/splitting | `"$variable"` not `$variable` |
| SC2046 | Quote to prevent word splitting | `"$(command)"` not `$(command)` |
| SC2034 | Variable appears unused | Remove or export it |
| SC2155 | Declare and assign separately | `local var; var=$(cmd)` not `local var=$(cmd)` |
| SC2162 | `read` without `-r` | Always `read -r` to avoid backslash mangling |
| SC2164 | Use `cd ... || exit` in case cd fails | `cd "$dir" || exit 1` |
| SC2128 | Expanding array without index | `"${arr[@]}"` not `"$arr"` |

### Inline Directives

```bash
# Disable for a specific line
# shellcheck disable=SC2034
unused_but_intentional="value"

# Disable for the entire file (top of file)
# shellcheck disable=SC2034,SC2086

# Specify the expected shell
# shellcheck shell=bash

# Source directives — tell shellcheck where sourced files are
# shellcheck source=lib/common.sh
source "${SCRIPT_DIR}/lib/common.sh"
```

### CI Integration

```yaml
# GitHub Actions
- name: ShellCheck
  uses: ludeeus/action-shellcheck@master
  with:
    scandir: './scripts'
    severity: warning
```

```makefile
# Makefile target
.PHONY: lint-sh
lint-sh:  ## Lint all shell scripts
	shellcheck scripts/*.sh
```

---

## Bats (Bash Automated Testing System)

Bats is a TAP-compliant testing framework for Bash. Use it for scripts with complex logic or reusable functions.

### Installation

```bash
# macOS
brew install bats-core

# Or clone with helpers
git clone https://github.com/bats-core/bats-core.git
git clone https://github.com/bats-core/bats-support.git
git clone https://github.com/bats-core/bats-assert.git
```

### Test File Structure

```bash
#!/usr/bin/env bats
# test/my_script.bats

# Load helpers (bats-support + bats-assert)
load 'test_helper/bats-support/load'
load 'test_helper/bats-assert/load'

# Setup runs before each test
setup() {
    # Source the script's functions (don't run main)
    source "${BATS_TEST_DIRNAME}/../lib/functions.sh"

    # Create temp dir for test artifacts
    TEST_TEMP_DIR="$(mktemp -d)"
}

# Teardown runs after each test
teardown() {
    rm -rf "$TEST_TEMP_DIR"
}

@test "validate_args rejects missing arguments" {
    run validate_args
    assert_failure
    assert_output --partial "Usage"
}

@test "validate_args accepts valid input file" {
    local test_file="${TEST_TEMP_DIR}/input.txt"
    echo "test content" > "$test_file"

    run validate_args "$test_file"
    assert_success
}

@test "process_log extracts error lines" {
    local log_file="${TEST_TEMP_DIR}/test.log"
    cat > "$log_file" <<'EOF'
2024-01-01 INFO Starting
2024-01-01 ERROR Connection failed
2024-01-01 INFO Retrying
2024-01-01 ERROR Timeout
EOF

    run process_log "$log_file" "$TEST_TEMP_DIR"
    assert_success

    # Verify output file
    assert [ -f "${TEST_TEMP_DIR}/errors.log" ]
    run cat "${TEST_TEMP_DIR}/errors.log"
    assert_line --index 0 --partial "Connection failed"
    assert_line --index 1 --partial "Timeout"
}
```

### Running Tests

```bash
# Run all tests
bats test/

# Run a specific file
bats test/my_script.bats

# With TAP output (for CI)
bats --tap test/

# Verbose/pretty output
bats --pretty test/

# Run tests matching a filter
bats --filter "validate" test/
```

### Bats Assertions (bats-assert)

| Assertion | Description |
|-----------|-------------|
| `assert_success` | Exit code is 0 |
| `assert_failure` | Exit code is non-zero |
| `assert_output "text"` | stdout equals text |
| `assert_output --partial "text"` | stdout contains text |
| `assert_line --index N "text"` | Line N of stdout equals text |
| `assert_line --partial "text"` | Any line contains text |
| `refute_output --partial "text"` | stdout does NOT contain text |
| `assert [ expression ]` | Assert a test expression |

---

## Writing Testable Scripts

Structure scripts so that logic lives in functions that can be sourced and tested independently.

### Pattern: Sourceable Functions + Main Guard

```bash
#!/usr/bin/env bash
set -euo pipefail

# --- Functions ---

validate_input() {
    local input="$1"
    [[ -n "$input" ]] || return 1
    [[ "$input" =~ ^[a-zA-Z0-9_-]+$ ]] || return 1
}

process_file() {
    local file="$1"
    local output_dir="$2"
    [[ -f "$file" ]] || return 1
    grep -i "error" "$file" > "${output_dir}/errors.log"
}

main() {
    local input_file="${1:?Usage: $0 <input_file>}"
    validate_input "$(basename "$input_file")" || die "Invalid filename"
    process_file "$input_file" "./output"
}

# --- Main guard ---
# Only run main if this script is executed, not sourced
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi
```

The `BASH_SOURCE` guard lets test files source the functions without triggering `main`:

```bash
# In test file:
setup() {
    source "${BATS_TEST_DIRNAME}/../my_script.sh"
}

@test "validate_input rejects empty string" {
    run validate_input ""
    assert_failure
}
```

---

## Project Layout

```
project/
├── scripts/
│   ├── deploy.sh
│   ├── setup.sh
│   └── lib/
│       └── common.sh         # Shared functions (sourced, not executed)
├── test/
│   ├── test_helper/
│   │   ├── bats-support/     # git submodule
│   │   └── bats-assert/      # git submodule
│   ├── deploy.bats
│   └── setup.bats
└── Makefile
```

```makefile
.PHONY: test-sh lint-sh
test-sh:  ## Run shell script tests
	bats test/

lint-sh:  ## Lint shell scripts
	shellcheck scripts/*.sh scripts/lib/*.sh
```

---

## CI Pipeline Example

```yaml
# GitHub Actions
name: Shell Scripts
on: [push, pull_request]

jobs:
  lint-and-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          submodules: true  # for bats helpers

      - name: Install tools
        run: |
          sudo apt-get update
          sudo apt-get install -y shellcheck
          sudo apt-get install -y bats

      - name: ShellCheck
        run: shellcheck scripts/*.sh

      - name: Bats tests
        run: bats test/
```
