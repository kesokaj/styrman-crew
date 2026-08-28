# Python Fundamentals Reference

## Import Conventions

### Import Style

```python
# ✅ Import packages and modules, not individual classes
import os
import sys
from collections import defaultdict
from typing import Optional

# ✅ Use full package paths, not relative imports
from myapp.services import user_service
from myapp.models import User

# ✅ Standard abbreviations are acceptable
import numpy as np
import pandas as pd
import tensorflow as tf

# ❌ Never use wildcard imports
from os import *         # Pollutes namespace
from typing import *     # Hides where types come from

# ❌ Do not import individual classes/functions from modules
from os.path import join, exists  # Use os.path.join instead
```

### Import Ordering

Always group imports in this order, separated by blank lines:

```python
# 1. Standard library
import os
import sys
from typing import Optional, Sequence

# 2. Third-party libraries
import requests
from fastapi import FastAPI

# 3. Local application modules
from myapp.config import settings
from myapp.models import User
```

---

## Exception Handling

### Catch Specific Exceptions

```python
# ❌ Never use bare except
try:
    result = risky_operation()
except:  # Catches KeyboardInterrupt, SystemExit, etc.
    pass

# ❌ Do not swallow exceptions silently
try:
    result = risky_operation()
except Exception:
    pass

# ✅ Catch the narrowest exception type
try:
    result = int(user_input)
except ValueError as e:
    logger.error(f"Invalid integer input: {e}")
    return default_value
```

### Exception Chaining

```python
# ✅ Always use `from` to preserve the cause
try:
    data = fetch_from_api()
except ConnectionError as e:
    raise ServiceUnavailableError("API unreachable") from e
```

### Custom Exceptions

```python
# ✅ End exception names with Error
# ✅ Do not repeat the module name (no foo.FooError)
class ValidationError(Exception):
    """Raised when input validation fails."""
    def __init__(self, field: str, message: str):
        self.field = field
        super().__init__(f"{field}: {message}")
```

### Minimize try/except Scope

```python
# ❌ Too much code in try block
try:
    data = fetch_data()
    processed = transform(data)
    result = save(processed)
except IOError:
    handle_error()

# ✅ Only the risky operation in try
data = fetch_data()
processed = transform(data)
try:
    result = save(processed)
except IOError:
    handle_error()
```

---

## Comprehensions

### One `for`, One `if` — Max

```python
# ✅ Simple and readable
result = [transform(x) for x in items if is_valid(x)]

# ✅ Multi-line for readability
result = [
    transform(x)
    for x in items
    if is_valid(x)
]

# ❌ Nested comprehensions — use a loop instead
result = [(x, y) for x in range(10) for y in range(5) if x * y > 10]

# ✅ Rewritten as a loop
result = []
for x in range(10):
    for y in range(5):
        if x * y > 10:
            result.append((x, y))
```

---

## Type Annotation Patterns

### Modern Syntax (Python 3.10+)

```python
# ✅ Use X | Y instead of Union[X, Y]
def find_user(user_id: int) -> User | None:
    ...

# ✅ Use X | None instead of Optional[X]
def get_name(user: User | None) -> str:
    ...

# ✅ Use built-in generics (Python 3.9+)
def process(items: list[str]) -> dict[str, int]:
    ...
```

### Protocol (Structural Subtyping)

```python
from typing import Protocol

class Serializable(Protocol):
    def to_json(self) -> str: ...

# Any class with a to_json method satisfies this — no inheritance needed
def save(obj: Serializable) -> None:
    data = obj.to_json()
    ...
```

---

## Decorator Guidelines

### When to Use Decorators

Decorators are appropriate for cross-cutting concerns applied to many functions: logging, authentication, caching, retry logic.

```python
# ✅ Clear, well-documented decorator
import functools

def retry(max_attempts: int = 3):
    """Retry a function up to max_attempts times on exception."""
    def decorator(func):
        @functools.wraps(func)
        def wrapper(*args, **kwargs):
            for attempt in range(max_attempts):
                try:
                    return func(*args, **kwargs)
                except Exception:
                    if attempt == max_attempts - 1:
                        raise
        return wrapper
    return decorator
```

### Always Use `functools.wraps`

Without `@functools.wraps`, the decorated function loses its name, docstring, and module — breaking debugging and documentation tools.

---

## True/False Evaluation

### Use Implicit Truthiness

```python
# ✅ Pythonic — use implicit truthiness
if items:          # Non-empty list
if not items:      # Empty list
if name:           # Non-empty string
if count:          # Non-zero number

# ❌ Verbose
if len(items) > 0:
if items != []:
if name != "":
if count != 0:

# ⚠️ Exception: when you need to distinguish None from False/0/""
if value is not None:  # Explicit None check
```

---

## String Handling

### Use f-strings

```python
# ✅ f-strings (preferred)
message = f"User {name} has {count} items"

# ❌ Old-style formatting
message = "User %s has %d items" % (name, count)
message = "User {} has {} items".format(name, count)
```

### Logging — Use Lazy Formatting

```python
import logging

# ✅ Lazy formatting — string is only built if the log level is enabled
logger.info("Processing user %s with %d items", user_id, count)

# ❌ Eager formatting — string is always built, even if not logged
logger.info(f"Processing user {user_id} with {count} items")
```

---

## Resource Management

### Always Use `with` for Closeable Resources

```python
# ✅ Files
with open("data.txt") as f:
    content = f.read()

# ✅ Database connections
with get_connection() as conn:
    conn.execute(query)

# ✅ Locks
with threading.Lock():
    shared_resource.update()

# ❌ Manual close — error-prone
f = open("data.txt")
content = f.read()
f.close()  # Skipped if an exception occurs above
```

---

## Main Guard

```python
# ✅ Every executable script must have this guard
def main():
    """Entry point."""
    ...

if __name__ == '__main__':
    main()
```

This prevents the script from executing when imported as a module, and keeps the main logic in a function (enabling testing and profiling).
