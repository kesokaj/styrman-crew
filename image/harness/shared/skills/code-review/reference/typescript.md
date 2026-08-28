# TypeScript/JavaScript Code Review Guide

> TypeScript code review guide covering the type system, generics, conditional types, strict mode, async/await patterns, immutability, module organization, and ESLint configuration.

## Table of Contents

- [Type Safety Fundamentals](#type-safety-fundamentals)
- [Generics](#generics)
- [Advanced Types](#advanced-types)
- [Strict Mode Configuration](#strict-mode-configuration)
- [Module Organization](#module-organization)
- [Async Patterns](#async-patterns)
- [Immutability](#immutability)
- [ESLint Rules](#eslint-rules)
- [Review Checklist](#review-checklist)

---

## Type Safety Fundamentals

### Avoid `any`

```typescript
// ❌ Using any defeats type safety
function processData(data: any) {
  return data.value;  // No type checking — may crash at runtime
}

// ✅ Use proper types
interface DataPayload {
  value: string;
}
function processData(data: DataPayload) {
  return data.value;
}

// ✅ Use unknown + type guard for truly unknown types
function processUnknown(data: unknown) {
  if (typeof data === 'object' && data !== null && 'value' in data) {
    return (data as { value: string }).value;
  }
  throw new Error('Invalid data');
}
```

### Type Narrowing

```typescript
// ❌ Unsafe type assertion
function getLength(value: string | string[]) {
  return (value as string[]).length;  // Breaks if value is a string
}

// ✅ Use type guards
function getLength(value: string | string[]): number {
  if (Array.isArray(value)) {
    return value.length;
  }
  return value.length;
}

// ✅ Use the `in` operator
interface Dog { bark(): void }
interface Cat { meow(): void }

function speak(animal: Dog | Cat) {
  if ('bark' in animal) {
    animal.bark();
  } else {
    animal.meow();
  }
}
```

### Literal Types and `as const`

```typescript
// ❌ Type is too wide
const config = {
  endpoint: '/api',
  method: 'GET'  // type is string
};

// ✅ Use as const for literal types
const config = {
  endpoint: '/api',
  method: 'GET'
} as const;  // method type is 'GET'

// ✅ Works with function parameters
function request(method: 'GET' | 'POST', url: string) { ... }
request(config.method, config.endpoint);  // Correct!
```

---

## Generics

### Basic Generics

```typescript
// ❌ Duplicated code
function getFirstString(arr: string[]): string | undefined {
  return arr[0];
}
function getFirstNumber(arr: number[]): number | undefined {
  return arr[0];
}

// ✅ Use generics
function getFirst<T>(arr: T[]): T | undefined {
  return arr[0];
}
```

### Generic Constraints

```typescript
// ❌ Unconstrained generic — cannot access properties
function getProperty<T>(obj: T, key: string) {
  return obj[key];  // Error: cannot index type T
}

// ✅ Use keyof constraint
function getProperty<T, K extends keyof T>(obj: T, key: K): T[K] {
  return obj[key];
}

const user = { name: 'Alice', age: 30 };
getProperty(user, 'name');  // return type is string
getProperty(user, 'age');   // return type is number
getProperty(user, 'foo');   // Error: 'foo' is not in keyof User
```

### Generic Defaults

```typescript
// ✅ Provide reasonable default types
interface ApiResponse<T = unknown> {
  data: T;
  status: number;
  message: string;
}

// Can omit the generic parameter
const response: ApiResponse = { data: null, status: 200, message: 'OK' };
// Or specify it
const userResponse: ApiResponse<User> = { ... };
```

### Built-in Utility Types

```typescript
// ✅ Use built-in utility types
interface User {
  id: number;
  name: string;
  email: string;
}

type PartialUser = Partial<User>;         // All properties optional
type RequiredUser = Required<User>;       // All properties required
type ReadonlyUser = Readonly<User>;       // All properties readonly
type UserKeys = keyof User;               // 'id' | 'name' | 'email'
type NameOnly = Pick<User, 'name'>;       // { name: string }
type WithoutId = Omit<User, 'id'>;        // { name: string; email: string }
type UserRecord = Record<string, User>;   // { [key: string]: User }
```

---

## Advanced Types

### Conditional Types

```typescript
// ✅ Return different types based on input type
type IsString<T> = T extends string ? true : false;

type A = IsString<string>;  // true
type B = IsString<number>;  // false

// ✅ Extract array element type
type ElementType<T> = T extends (infer U)[] ? U : never;

type Elem = ElementType<string[]>;  // string

// ✅ Extract function return type (built-in ReturnType)
type MyReturnType<T> = T extends (...args: any[]) => infer R ? R : never;
```

### Mapped Types

```typescript
// ✅ Transform all properties of an object type
type Nullable<T> = {
  [K in keyof T]: T[K] | null;
};

interface User {
  name: string;
  age: number;
}

type NullableUser = Nullable<User>;
// { name: string | null; age: number | null }

// ✅ Add prefixes
type Getters<T> = {
  [K in keyof T as `get${Capitalize<string & K>}`]: () => T[K];
};

type UserGetters = Getters<User>;
// { getName: () => string; getAge: () => number }
```

### Template Literal Types

```typescript
// ✅ Type-safe event names
type EventName = 'click' | 'focus' | 'blur';
type HandlerName = `on${Capitalize<EventName>}`;
// 'onClick' | 'onFocus' | 'onBlur'

// ✅ API route types
type ApiRoute = `/api/${string}`;
const route: ApiRoute = '/api/users';  // OK
const badRoute: ApiRoute = '/users';   // Error
```

### Discriminated Unions

```typescript
// ✅ Use a discriminant property for type-safe branching
type Result<T, E> =
  | { success: true; data: T }
  | { success: false; error: E };

function handleResult(result: Result<User, Error>) {
  if (result.success) {
    console.log(result.data.name);  // TypeScript knows data exists
  } else {
    console.log(result.error.message);  // TypeScript knows error exists
  }
}

// ✅ Redux action pattern
type Action =
  | { type: 'INCREMENT'; payload: number }
  | { type: 'DECREMENT'; payload: number }
  | { type: 'RESET' };

function reducer(state: number, action: Action): number {
  switch (action.type) {
    case 'INCREMENT':
      return state + action.payload;  // payload type is known
    case 'DECREMENT':
      return state - action.payload;
    case 'RESET':
      return 0;  // no payload here
  }
}
```

---

## Strict Mode Configuration

### Recommended tsconfig.json

```json
{
  "compilerOptions": {
    // ✅ Required strict options
    "strict": true,
    "noImplicitAny": true,
    "strictNullChecks": true,
    "strictFunctionTypes": true,
    "strictBindCallApply": true,
    "strictPropertyInitialization": true,
    "noImplicitThis": true,
    "useUnknownInCatchVariables": true,

    // ✅ Additional recommended options
    "noUncheckedIndexedAccess": true,
    "noImplicitReturns": true,
    "noFallthroughCasesInSwitch": true,
    "exactOptionalPropertyTypes": true,
    "noPropertyAccessFromIndexSignature": true
  }
}
```

### Impact of noUncheckedIndexedAccess

```typescript
// tsconfig: "noUncheckedIndexedAccess": true

const arr = [1, 2, 3];
const first = arr[0];  // type is number | undefined

// ❌ Might be undefined
console.log(first.toFixed(2));  // Error: possibly undefined

// ✅ Check first
if (first !== undefined) {
  console.log(first.toFixed(2));
}

// ✅ Or use non-null assertion (when certain)
console.log(arr[0]!.toFixed(2));
```

---

## Module Organization

Rules for structuring imports and exports, adapted from Google's TypeScript Style Guide.

### No Default Exports

Default exports allow inconsistent naming across importing modules. Use named exports exclusively.

```typescript
// ❌ Default exports — importers can name it anything
export default class UserService { ... }
// import Foo from './user-service';  // Unclear what Foo is

// ✅ Named exports — import name matches the declaration
export class UserService { ... }
// import { UserService } from './user-service';  // Unambiguous
```

### Use `import type` for Type-Only Imports

Separate type imports from value imports. This makes the runtime dependency graph explicit and enables bundlers to tree-shake more effectively.

```typescript
// ❌ Mixing value and type imports
import { UserService, User } from './user-service';

// ✅ Separate type imports
import { UserService } from './user-service';
import type { User } from './user-service';

// ✅ Inline type imports (TypeScript 4.5+)
import { UserService, type User } from './user-service';
```

### No Mutable Exports

Do not export `let` bindings. Exported variables must be `const`. If consumers need mutable state, expose it through getter/setter functions.

```typescript
// ❌ Mutable export — consumers see stale values
export let count = 0;
export function increment() { count++; }

// ✅ Encapsulate mutable state
let count = 0;
export function getCount() { return count; }
export function increment() { count++; }
```

### No Namespaces — Use Modules

TypeScript namespaces (`namespace Foo { ... }`) are a legacy pattern. Use ES modules (`import`/`export`) for all code organization.

```typescript
// ❌ Namespace — legacy pattern
namespace Validators {
  export function isEmail(s: string): boolean { ... }
}

// ✅ Module — modern pattern
// validators.ts
export function isEmail(s: string): boolean { ... }
```

### No `require()` — Use ES Module Syntax

Always use `import`/`export` statements. Never use `require()` or `module.exports` in TypeScript.

```typescript
// ❌ CommonJS
const fs = require('fs');
module.exports = { myFunction };

// ✅ ES Modules
import fs from 'node:fs';
export { myFunction };
```

### No Container Classes

Do not create classes that only hold static members as a way to namespace functions. Use module scope instead.

```typescript
// ❌ Container class for namespacing
export class StringUtils {
  static capitalize(s: string): string { ... }
  static truncate(s: string, len: number): string { ... }
}

// ✅ Module-scoped functions
export function capitalize(s: string): string { ... }
export function truncate(s: string, len: number): string { ... }
```

---

## Async Patterns

### Promise Error Handling

```typescript
// ❌ Not handling async errors
async function fetchUser(id: string) {
  const response = await fetch(`/api/users/${id}`);
  return response.json();  // Network errors not handled
}

// ✅ Handle errors properly
async function fetchUser(id: string): Promise<User> {
  try {
    const response = await fetch(`/api/users/${id}`);
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}: ${response.statusText}`);
    }
    return await response.json();
  } catch (error) {
    if (error instanceof Error) {
      throw new Error(`Failed to fetch user: ${error.message}`);
    }
    throw error;
  }
}
```

### Promise.all vs Promise.allSettled

```typescript
// ❌ Promise.all — one failure rejects everything
async function fetchAllUsers(ids: string[]) {
  const users = await Promise.all(ids.map(fetchUser));
  return users;  // One failure aborts all
}

// ✅ Promise.allSettled — collects all results
async function fetchAllUsers(ids: string[]) {
  const results = await Promise.allSettled(ids.map(fetchUser));

  const users: User[] = [];
  const errors: Error[] = [];

  for (const result of results) {
    if (result.status === 'fulfilled') {
      users.push(result.value);
    } else {
      errors.push(result.reason);
    }
  }

  return { users, errors };
}
```

### Race Condition Handling

```typescript
// ❌ Race condition: old request may overwrite new one
function useSearch() {
  const [query, setQuery] = useState('');
  const [results, setResults] = useState([]);

  useEffect(() => {
    fetch(`/api/search?q=${query}`)
      .then(r => r.json())
      .then(setResults);  // Old request may return last!
  }, [query]);
}

// ✅ Use AbortController
function useSearch() {
  const [query, setQuery] = useState('');
  const [results, setResults] = useState([]);

  useEffect(() => {
    const controller = new AbortController();

    fetch(`/api/search?q=${query}`, { signal: controller.signal })
      .then(r => r.json())
      .then(setResults)
      .catch(e => {
        if (e.name !== 'AbortError') throw e;
      });

    return () => controller.abort();
  }, [query]);
}
```

---

## Immutability

### Readonly and ReadonlyArray

```typescript
// ❌ Mutable parameters can be accidentally modified
function processUsers(users: User[]) {
  users.sort((a, b) => a.name.localeCompare(b.name));  // Mutates the original array!
  return users;
}

// ✅ Use readonly to prevent mutation
function processUsers(users: readonly User[]): User[] {
  return [...users].sort((a, b) => a.name.localeCompare(b.name));
}

// ✅ Deep readonly
type DeepReadonly<T> = {
  readonly [K in keyof T]: T[K] extends object ? DeepReadonly<T[K]> : T[K];
};
```

### Immutable Function Parameters

```typescript
// ✅ Use as const with readonly to protect data
function createConfig<T extends readonly string[]>(routes: T) {
  return routes;
}

const routes = createConfig(['home', 'about', 'contact'] as const);
// type is readonly ['home', 'about', 'contact']
```

---

## ESLint Rules

### Recommended @typescript-eslint Rules

```javascript
// .eslintrc.js
module.exports = {
  extends: [
    'eslint:recommended',
    'plugin:@typescript-eslint/recommended',
    'plugin:@typescript-eslint/recommended-requiring-type-checking',
    'plugin:@typescript-eslint/strict'
  ],
  rules: {
    // ✅ Type safety
    '@typescript-eslint/no-explicit-any': 'error',
    '@typescript-eslint/no-unsafe-assignment': 'error',
    '@typescript-eslint/no-unsafe-member-access': 'error',
    '@typescript-eslint/no-unsafe-call': 'error',
    '@typescript-eslint/no-unsafe-return': 'error',

    // ✅ Best practices
    '@typescript-eslint/explicit-function-return-type': 'warn',
    '@typescript-eslint/no-floating-promises': 'error',
    '@typescript-eslint/await-thenable': 'error',
    '@typescript-eslint/no-misused-promises': 'error',

    // ✅ Code style
    '@typescript-eslint/consistent-type-imports': 'error',
    '@typescript-eslint/prefer-nullish-coalescing': 'error',
    '@typescript-eslint/prefer-optional-chain': 'error'
  }
};
```

### Common ESLint Error Fixes

```typescript
// ❌ no-floating-promises: Promise must be handled
async function save() { ... }
save();  // Error: unhandled Promise

// ✅ Handle explicitly
await save();
// or
save().catch(console.error);
// or explicitly ignore
void save();

// ❌ no-misused-promises: cannot use Promise in non-async position
const items = [1, 2, 3];
items.forEach(async (item) => {  // Error!
  await processItem(item);
});

// ✅ Use for...of
for (const item of items) {
  await processItem(item);
}
// or Promise.all
await Promise.all(items.map(processItem));
```

---

## Review Checklist

### Type System
- [ ] No usage of `any` (use `unknown` + type guards instead)
- [ ] Interfaces and type definitions are complete with meaningful names
- [ ] Generics are used to improve code reuse
- [ ] Union types have proper type narrowing
- [ ] Built-in utility types are used where appropriate (Partial, Pick, Omit, etc.)

### Generics
- [ ] Generics have appropriate constraints (`extends`)
- [ ] Generic parameters have reasonable defaults
- [ ] No over-generalization (KISS principle)

### Strict Mode
- [ ] tsconfig.json has `strict: true` enabled
- [ ] `noUncheckedIndexedAccess` is enabled
- [ ] No `@ts-ignore` usage (use `@ts-expect-error` instead)

### Module Organization
- [ ] No default exports — named exports only
- [ ] `import type` used for type-only imports
- [ ] No mutable exports (`export let`)
- [ ] No namespaces — ES modules only
- [ ] No container classes for namespacing

### Async Code
- [ ] Async functions have error handling
- [ ] Promise rejections are properly handled
- [ ] No floating promises (unhandled Promises)
- [ ] Concurrent requests use Promise.all or Promise.allSettled
- [ ] Race conditions handled with AbortController

### Immutability
- [ ] Function parameters are not directly mutated
- [ ] Spread operators used to create new objects/arrays
- [ ] `readonly` modifiers used where appropriate

### ESLint
- [ ] Using @typescript-eslint/recommended
- [ ] No ESLint warnings or errors
- [ ] Using consistent-type-imports
