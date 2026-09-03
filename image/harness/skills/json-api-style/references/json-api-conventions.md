# JSON API Conventions Reference

## Property Naming

### Rules

- Property names MUST be `camelCase` ASCII strings
- The first character MUST be a letter, underscore (`_`), or dollar sign (`$`)
- Subsequent characters may include digits
- Avoid JavaScript reserved words (`class`, `default`, `export`, etc.)

```json
{
  "firstName": "Alice",
  "lastName": "Smith",
  "emailAddress": "alice@example.com",
  "createdAt": "2024-01-15T09:30:00Z"
}
```

### Singular vs Plural

Array properties get plural names. All other properties get singular names.

```json
{
  "userName": "alice",
  "totalItems": 42,
  "items": [
    { "id": 1, "name": "Widget" },
    { "id": 2, "name": "Gadget" }
  ]
}
```

### Map Keys

When a JSON object is used as a map (arbitrary key-value pairs), the keys do NOT need to follow camelCase rules. Document this in the API spec.

```json
{
  "thumbnails": {
    "72": "https://cdn.example.com/thumb/72.png",
    "144": "https://cdn.example.com/thumb/144.png",
    "288": "https://cdn.example.com/thumb/288.png"
  }
}
```

---

## Data Structure

### Flat vs Structured

Default to flat properties. Only nest into sub-objects when the grouping is semantically meaningful.

✅ **Flat** — simple, predictable:

```json
{
  "company": "Acme Corp",
  "website": "https://acme.example.com",
  "street": "123 Main St",
  "city": "Springfield",
  "state": "IL",
  "zip": "62701"
}
```

✅ **Structured** — when the grouping is a real entity:

```json
{
  "company": "Acme Corp",
  "website": "https://acme.example.com",
  "address": {
    "street": "123 Main St",
    "city": "Springfield",
    "state": "IL",
    "zip": "62701"
  }
}
```

---

## Property Value Types

### Allowed Types

JSON property values MUST be one of: boolean, number, string, object, array, or `null`.

```json
{
  "isActive": true,
  "count": 42,
  "name": "Alice",
  "metadata": {},
  "tags": [],
  "deletedAt": null
}
```

### Empty and Null Values

Omit optional properties that have `null` or empty values, unless the absence would be semantically ambiguous.

✅ **Omit null** — the client knows it's absent:

```json
{
  "volume": 10,
  "balance": 0
}
```

✅ **Keep null** when absence vs. null has different meaning:

```json
{
  "volume": 10,
  "balance": 0,
  "currentTrack": null
}
```

### Enum Values

Enums MUST be represented as strings, never integers. Strings survive schema evolution (adding, removing, renaming values) without breaking clients.

```json
{
  "status": "ACTIVE",
  "priority": "HIGH",
  "role": "ADMIN"
}
```

---

## Date, Time, and Duration Formats

### Dates and Timestamps — RFC 3339

```json
{
  "createdAt": "2024-01-15T09:30:00.000Z",
  "updatedAt": "2024-06-20T14:22:33.456Z",
  "expiresAt": "2025-12-31T23:59:59Z"
}
```

### Durations — ISO 8601

```json
{
  "sessionTimeout": "PT30M",
  "retentionPeriod": "P90D",
  "processingTime": "PT2H15M30S"
}
```

| Duration | ISO 8601 |
|----------|----------|
| 30 minutes | `PT30M` |
| 2 hours, 15 minutes | `PT2H15M` |
| 90 days | `P90D` |
| 1 year, 6 months | `P1Y6M` |

### Latitude/Longitude — ISO 6709

```json
{
  "location": "+48.8566+002.3522"
}
```

---

## Reserved Property Names

These property names have conventional meanings across Google APIs. Use them consistently when applicable.

### Top-Level Response Properties

| Property | Type | Description |
|----------|------|-------------|
| `apiVersion` | string | Version of the API |
| `context` | string | Client-provided context returned unchanged |
| `id` | string | Unique identifier for the resource |
| `method` | string | HTTP method or RPC method name |
| `data` | object | Primary response data container |
| `error` | object | Error information container |

### Data Object Properties

| Property | Type | Description |
|----------|------|-------------|
| `kind` | string | Type of resource (e.g., `"user"`, `"product"`) |
| `items` | array | Collection of resources |
| `currentItemCount` | integer | Number of items in this response page |
| `totalItems` | integer | Total items across all pages |
| `itemsPerPage` | integer | Max items per page |
| `startIndex` | integer | 1-based index of the first item in this page |
| `pageIndex` | integer | Current page index |
| `totalPages` | integer | Total number of pages |
| `nextLink` | string | URL to the next page |
| `previousLink` | string | URL to the previous page |
| `selfLink` | string | URL to this resource |
| `etag` | string | ETag for caching |
| `lang` | string | BCP 47 language code |
| `updated` | string | RFC 3339 timestamp of last update |
| `deleted` | boolean | Whether the resource is soft-deleted |

---

## Pagination

### Token-Based Pagination (Preferred)

**Request:** `GET /api/users?pageToken=abc123&maxResults=25`

**Response:**

```json
{
  "kind": "userList",
  "items": [],
  "currentItemCount": 25,
  "totalItems": 150,
  "nextPageToken": "def456"
}
```

### Offset-Based Pagination

**Request:** `GET /api/users?startIndex=26&maxResults=25`

**Response:**

```json
{
  "kind": "userList",
  "items": [],
  "currentItemCount": 25,
  "totalItems": 150,
  "startIndex": 26,
  "itemsPerPage": 25,
  "nextLink": "/api/users?startIndex=51&maxResults=25"
}
```

---

## Error Object Structure

### Standard Error Response

```json
{
  "error": {
    "code": 404,
    "message": "User not found",
    "errors": [
      {
        "domain": "users",
        "reason": "notFound",
        "message": "User with id '123' does not exist"
      }
    ]
  }
}
```

### Error Properties

| Property | Type | Description |
|----------|------|-------------|
| `error.code` | integer | HTTP status code |
| `error.message` | string | Human-readable summary |
| `error.errors` | array | Detailed error list |
| `error.errors[].domain` | string | Service or subsystem |
| `error.errors[].reason` | string | Machine-readable error code |
| `error.errors[].message` | string | Detailed human-readable description |
| `error.errors[].location` | string | Which field caused the error |
| `error.errors[].locationType` | string | Type of location (`"header"`, `"parameter"`, `"jsonPath"`) |

---

## Naming Conflicts

If a new reserved property name conflicts with an existing property in your API:

1. **Rename your property** to avoid the collision
2. Or **version the API** on a major version boundary

**Before** — `ingredients` is your property:

```json
{
  "apiVersion": "1.0",
  "data": {
    "recipeName": "pizza",
    "ingredients": ["tomatoes", "cheese"]
  }
}
```

**After** — renamed to avoid future conflict:

```json
{
  "apiVersion": "2.0",
  "data": {
    "recipeName": "pizza",
    "recipeIngredients": ["tomatoes", "cheese"]
  }
}
```
