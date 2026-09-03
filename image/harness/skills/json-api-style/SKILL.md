---
name: json-api-style
description: |
  JSON API response and request design conventions. Use when designing JSON API payloads,
  naming JSON properties, structuring error responses, or defining date/time formats in APIs.
  Activate for: JSON property naming, API response structure, pagination design,
  error object format, enum representation, null handling, camelCase conventions.
  Do NOT use for: JSON parsing/serialization code, JSON Schema validation, or YAML/TOML config files.
---

# JSON API Style

Conventions for designing consistent, predictable JSON API request and response payloads. Based on the Google JSON Style Guide.

## When to Use

- Designing JSON API response payloads
- Naming properties in JSON objects
- Structuring error responses
- Defining date, time, or enum formats
- Reviewing API contracts and specs

## When NOT to Use

- JSON parsing/serialization code (use language-specific skills)
- JSON Schema or OpenAPI spec writing (use API spec docs)
- YAML, TOML, or other config file formats
- Frontend state management shapes

## Core Rules

1. **Property names: `camelCase`** — always. Meaningful, no JavaScript reserved words.
2. **Arrays get plural names**, scalars get singular: `"items": [...]`, `"name": "..."`.
3. **Flat by default** — only nest into sub-objects when the grouping is semantically meaningful (e.g., an `address` object).
4. **No comments in JSON** — JSON does not support comments.
5. **Double quotes always** — for both property names and string values.
6. **Drop optional nulls** — omit properties with `null` values unless the absence would be ambiguous.
7. **Enums: always strings** — never integers. Strings survive schema evolution gracefully.
8. **Dates: RFC 3339** — `"2024-11-06T16:34:41.000Z"`.
9. **Durations: ISO 8601** — `"P1DT2H3M4.5S"` (1 day, 2 hours, 3 minutes, 4.5 seconds).
10. **Latitude/Longitude: ISO 6709** — `"+48.8566+002.3522"`.

## Reference Library

For detailed patterns, examples, reserved property names, and error object structure:

* **JSON API Conventions (property naming, error objects, pagination, reserved names)**
  Read → `./references/json-api-conventions.md`

> [!TIP]
> When designing an API response, read the reference to check property naming conventions and the standard error object structure before writing code.

## Further Reading

- [Google JSON Style Guide](https://google.github.io/styleguide/jsoncstyleguide.xml) — Google's internal JSON API conventions (used as the foundation for this skill)
