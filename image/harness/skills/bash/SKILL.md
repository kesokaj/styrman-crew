---
name: bash
description: "Bash and shell scripting expert. Use when writing, reviewing, or debugging Bash/Zsh scripts, automating tasks with shell commands, or using CLI tools like jq, rg, fd, fzf, yq. Do NOT use for general application backend code (use golang) or frontend code (use frontend/typescript)."
allowed-tools: Bash
---

# Bash & Shell Scripting

Expert guidance for writing robust, maintainable, and secure shell scripts following modern best practices.

## When to Use

- Writing or reviewing Bash/Zsh scripts
- Automating tasks with shell commands
- Argument parsing, error handling, or signal traps
- Using CLI tools: `jq`, `rg`, `fd`, `fzf`, `yq`, `bat`
- POSIX portability questions

## When NOT to Use

- Application logic that belongs in Go, Python, or TypeScript
- Terraform/IaC configuration (use the `terraform` skill)
- CI/CD pipeline design beyond shell steps (use platform-specific docs)

## When to Stop Using Shell

Shell is the right tool for glue scripts and automation. Switch to Go or Python when:

- The script exceeds **~100 lines** of non-trivial control flow (conditionals, loops, data transformation)
- **Performance matters** — shell is slow for anything compute-heavy
- You need **data structures** beyond arrays and strings (maps, sets, structs)
- **Error handling** requires more granularity than exit codes
- **Multiple developers** will maintain the script — shell is harder to read at scale

If the script is mostly calling other utilities with little data manipulation, shell is fine regardless of length.

## Core Rules

1. Always use `#!/usr/bin/env bash` and `set -euo pipefail`
2. Quote all variables: `"$variable"`, never `$variable`
3. Use `[[ ]]` for conditionals, `(( ))` for arithmetic
4. Use `local` for function variables, `readonly` for constants
5. Clean up with `trap cleanup EXIT`
6. Validate all inputs before use
7. Use `$()` for command substitution, never backticks
8. Never use `eval` — build commands with arrays instead

## Reference Library

For detailed patterns, examples, and templates, consult the reference files:

* **Script Foundations, Variables, Error Handling, Functions, Conditionals, Loops, Arrays**
  Read → `./references/bash-fundamentals.md`

* **Testing, ShellCheck, Linting, CI Validation**
  Read → `./references/testing-linting.md`

* **Makefile Patterns, Targets, Conventions**
  Read → `./references/makefile-patterns.md`

> [!TIP]
> When the user asks you to write a shell script, immediately read the reference file to apply correct patterns for error handling, argument parsing, and cleanup.

## Further Reading

- [Google Shell Style Guide](https://google.github.io/styleguide/shellguide.html) — Google's internal shell conventions (used as a reference for function documentation and style)
