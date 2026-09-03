---
name: python
description: |
  Python coding standards and best practices expert. Use when writing, reviewing, or debugging Python code,
  building Python applications, working with type annotations, async/await patterns, or pytest.
  Activate for any Python code question — even simple one-liners or quick type annotation checks.
  Do NOT use for frontend work (use the frontend skill), Go code (use golang), or infrastructure (use terraform).
---

# Python

Expert guidance for writing robust, maintainable, and type-safe Python code following Google-style conventions.

## When to Use

- Writing or reviewing Python code
- Type annotation questions
- Async/await patterns
- pytest test design
- Python packaging and project structure
- Performance optimization

## When NOT to Use

- Frontend-only work (use the `frontend` skill)
- Go code (use the `golang` skill)
- Shell scripts (use the `bash` skill)
- Infrastructure-as-code (use the `terraform` skill)

## Core Rules

1. **Always type-annotate** function signatures — parameters and return types. Prefer Python 3.10+ `X | Y` syntax over `Union[X, Y]`.
2. **Use `ruff`** (preferred) or `pylint` for linting. Run before every commit.
3. **Comprehensions:** one `for` clause, one optional `if` — no nested comprehensions. If it doesn't fit one line clearly, use a loop.
4. **Imports:** `import x` for packages/modules, `from x import y` for submodules. Never `from x import *`. Use full package paths, no relative imports.
5. **No mutable default arguments** — use `None` sentinel: `def f(items=None): items = items or []`.
6. **No mutable global state** — pass dependencies explicitly or use dependency injection.
7. **Always use `with`** for files, sockets, database connections, and any closeable resource.
8. **Google-style docstrings** with `Args:`, `Returns:`, `Raises:` sections for all public functions.
9. **Naming:**
   - `snake_case` for functions, methods, and variables
   - `CamelCase` for classes
   - `UPPER_CASE` for module-level constants
   - `_leading_underscore` for private/internal names
10. **`if __name__ == '__main__':` guard** in all executable scripts.
11. **Function length:** ≤40 lines preferred. If longer, extract helpers.
12. **No `assert` for validation** — use explicit `if`/`raise`. `assert` is stripped in optimized mode.
13. **Use f-strings** for string formatting. Not `%` or `.format()`. **Exception:** use lazy `%` formatting in logging calls (`logger.info("user %s", name)`) — the string is only built if the log level is enabled.
14. **Use `is` only for `None` and singletons** — `==` for value comparison.

## Reference Library

For detailed patterns, examples, and templates, consult the reference files:

* **Fundamentals (Imports, Exceptions, Types, Comprehensions, Decorators, Threading)**
  Read → `./references/python-fundamentals.md`

* **Testing (pytest, Fixtures, Mocking, Coverage)**
  Read → `./references/python-testing.md`

> [!TIP]
> When writing Python code, immediately read the relevant reference file to apply correct patterns for type annotations, exception handling, and testing.

## Further Reading

- [Google Python Style Guide](https://google.github.io/styleguide/pyguide.html) — Google's internal Python conventions (used as inspiration, not adopted wholesale)
