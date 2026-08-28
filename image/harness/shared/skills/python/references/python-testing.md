# Python Testing Reference

## pytest Patterns

### Test Naming

Test functions MUST clearly describe the scenario and expected outcome:

```python
# ✅ Descriptive names
def test_create_user_with_valid_email_succeeds():
    ...

def test_create_user_with_duplicate_email_raises_validation_error():
    ...

def test_delete_user_returns_none_when_user_not_found():
    ...

# ❌ Vague names
def test_user():
    ...

def test_error():
    ...
```

### Parametrized Tests

```python
import pytest

@pytest.mark.parametrize("input_val, expected", [
    ("hello", "HELLO"),
    ("World", "WORLD"),
    ("", ""),
    ("123abc", "123ABC"),
])
def test_uppercase(input_val: str, expected: str):
    assert input_val.upper() == expected
```

### Testing Exceptions

```python
def test_divide_by_zero_raises():
    with pytest.raises(ZeroDivisionError, match="division by zero"):
        1 / 0

def test_invalid_input_raises_validation_error():
    with pytest.raises(ValidationError) as exc_info:
        validate_email("not-an-email")
    assert exc_info.value.field == "email"
```

---

## Fixtures

### Basic Fixtures

```python
@pytest.fixture
def sample_user() -> User:
    return User(name="Test User", email="test@example.com")

def test_user_name(sample_user: User):
    assert sample_user.name == "Test User"
```

### Fixtures with Cleanup

```python
from typing import Generator

@pytest.fixture
def database() -> Generator[Database, None, None]:
    db = Database()
    db.connect()
    yield db
    db.disconnect()  # Runs after the test
```

### Fixture Scopes

```python
@pytest.fixture(scope="session")
def app():
    """Shared across the entire test session — created once."""
    return create_app()

@pytest.fixture(scope="module")
def db_connection(app):
    """Shared per test module."""
    return app.create_db_connection()

@pytest.fixture  # Default: scope="function"
def fresh_user():
    """Created fresh for each test function."""
    return User(name="Fresh")
```

### conftest.py

Place shared fixtures in `conftest.py` at the appropriate directory level. pytest discovers them automatically — no imports needed.

```
tests/
├── conftest.py          # Session/module-scoped fixtures
├── test_users.py
├── integration/
│   ├── conftest.py      # Integration-specific fixtures
│   └── test_api.py
```

---

## Mocking

### Mock External Dependencies

```python
from unittest.mock import Mock, patch, ANY

def test_send_notification():
    mock_client = Mock()
    mock_client.send.return_value = True

    service = NotificationService(client=mock_client)
    result = service.notify("user@example.com", "Hello")

    assert result is True
    mock_client.send.assert_called_once_with(
        to="user@example.com",
        message="Hello",
    )
```

### Patch Module-Level Functions

```python
@patch("myapp.services.external_api.fetch")
def test_with_patched_api(mock_fetch):
    mock_fetch.return_value = {"status": "ok", "data": [1, 2, 3]}

    result = process_external_data()

    assert result == [1, 2, 3]
    mock_fetch.assert_called_once()
```

### Async Mocking

```python
from unittest.mock import AsyncMock, patch

async def test_async_fetch():
    mock_fetch = AsyncMock(return_value={"data": "test"})

    with patch("myapp.client.fetch", mock_fetch):
        result = await get_data()

    assert result == {"data": "test"}
```

### When NOT to Mock

- **Pure functions** — test with real inputs/outputs
- **Data classes** — construct real instances
- **Standard library** — don't mock `os.path.join` or `json.dumps`

Mock at the boundary: network calls, databases, external APIs, filesystem I/O in integration tests.

---

## Async Testing

### pytest-asyncio

```python
import pytest

@pytest.mark.asyncio
async def test_fetch_user():
    user = await fetch_user(user_id=1)
    assert user.name == "Alice"

@pytest.fixture
async def async_client():
    async with AsyncClient(app=app) as client:
        yield client

@pytest.mark.asyncio
async def test_api_endpoint(async_client):
    response = await async_client.get("/api/users")
    assert response.status_code == 200
```

---

## Coverage Configuration

### pyproject.toml

```toml
[tool.pytest.ini_options]
asyncio_mode = "auto"
addopts = "--cov=myapp --cov-report=term-missing --cov-fail-under=80"
testpaths = ["tests"]
markers = [
    "slow: marks tests as slow (deselect with '-m \"not slow\"')",
    "integration: marks integration tests",
]

[tool.coverage.run]
branch = true
omit = ["tests/*", "*/migrations/*"]

[tool.coverage.report]
exclude_lines = [
    "pragma: no cover",
    "if TYPE_CHECKING:",
    "if __name__ == .__main__.:",
]
```

### Running Tests

```bash
# Run all tests
pytest

# Run with verbose output
pytest -v

# Run specific test file
pytest tests/test_users.py

# Run specific test function
pytest tests/test_users.py::test_create_user

# Skip slow tests
pytest -m "not slow"

# Run only integration tests
pytest -m integration

# Show coverage report
pytest --cov=myapp --cov-report=html
```

---

## Test Organization

### Arrange-Act-Assert Pattern

```python
def test_user_full_name():
    # Arrange
    user = User(first_name="John", last_name="Doe")

    # Act
    full_name = user.full_name()

    # Assert
    assert full_name == "John Doe"
```

### Group Related Tests in Classes

```python
class TestUserCreation:
    def test_valid_email(self):
        user = User(email="valid@example.com")
        assert user.email == "valid@example.com"

    def test_invalid_email_raises(self):
        with pytest.raises(ValidationError):
            User(email="not-valid")

    def test_duplicate_email_raises(self, existing_user):
        with pytest.raises(DuplicateError):
            User(email=existing_user.email)
```

### Test Boundary Cases

Always test:
- Empty inputs (`[]`, `""`, `{}`, `None`)
- Single-element inputs
- Maximum/minimum values
- Invalid types
- Concurrent access (for async/threaded code)
