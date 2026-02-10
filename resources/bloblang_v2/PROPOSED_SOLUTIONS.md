# Bloblang Language Improvement Proposals

**Date:** 2026-02-10
**Status:** Draft proposals for discussion
**Reference:** See BLOBLANG_ISSUES.md for detailed problem descriptions

This document proposes concrete solutions for each identified issue in the Bloblang language.

---

## Solution 1: Block-Scoped Variable Declarations

### Problem Reference
Issue #1: Variable Scoping Limitations

### Proposed Solution
Allow `let` declarations inside conditional blocks, match expressions, and as part of expression sequences.

### Syntax Proposal

```bloblang
# Option A: Block-scoped let (similar to JavaScript/Rust)
root.age = if this.birthdate != null {
  let parsed = this.birthdate.ts_parse("2006-01-02")
  $parsed.ts_unix()
} else {
  null
}

# Option B: Expression-level binding with 'let...in' syntax
root.age = if this.birthdate != null {
  let parsed = this.birthdate.ts_parse("2006-01-02") in $parsed.ts_unix()
}

# Option C: Inline binding operator (->)
root.age = if this.birthdate != null {
  this.birthdate.ts_parse("2006-01-02") -> parsed | $parsed.ts_unix()
}
```

### Recommendation
**Option A** - Most intuitive, aligns with common programming languages.

### Implementation Considerations
- Variables scoped to the block where declared
- Variables cannot leak outside their declaring block
- Shadow outer variables allowed (or disallowed for safety?)
- Parser changes to track scope depth
- Runtime changes to manage variable stack frames

### Backward Compatibility
✅ **Fully backward compatible** - Only adds new capability, doesn't change existing behavior

### Examples

```bloblang
# Complex transformation with scoped variables
root.processed = match this.type {
  "timestamp" => {
    let fmt = this.format.or("2006-01-02")
    let parsed = this.value.ts_parse($fmt)
    $parsed.ts_unix()
  }
  "number" => {
    let val = this.value.number()
    let multiplier = this.multiplier.or(1)
    $val * $multiplier
  }
  _ => this.value
}

# Nested scopes
root.result = if this.enabled {
  let outer = this.value
  if $outer > 100 {
    let inner = $outer / 2  # Scoped to inner if
    $inner.floor()
  } else {
    $outer  # Can still access outer
  }
}
```

---

## Solution 2: Explicit Context Management

### Problem Reference
Issue #2: Context Shifting Complexity

### Proposed Solution
Introduce explicit context binding and deprecate implicit context shifting in `match` expressions.

### Syntax Proposal

```bloblang
# Option A: Always require named context in match
match this.pet as pet {
  pet == "cat" => 3
  pet == "dog" => 5
  _ => 0
}

# Option B: Keep current syntax but add 'as' for clarity (opt-in)
match this.pet {
  this == "cat" => 3      # Current behavior: 'this' refers to this.pet
  this == "dog" => 5
  _ => 0
}

# With explicit binding (clearer)
match this.pet as pet {
  pet == "cat" => 3       # Explicit: 'pet' refers to matched value
  pet == "dog" => 5
  _ => 0
}

# Option C: Use different keyword for context matching
inspect this.pet {
  "cat" => 3
  "dog" => 5
  _ => 0
}
```

### Recommendation
**Option A** - Require explicit naming to eliminate ambiguity. Provide migration path via linter warnings.

### Additional: Stable Context in Error Handlers

```bloblang
# Problem: Context unreliable in catch chains
root.parsed = this.date.ts_parse("2006-01-02").catch(
  this.date.ts_parse("2006/01/02")  # this.date might not work
)

# Solution: Preserve original context in catch blocks
root.parsed = this.date.ts_parse("2006-01-02").catch(
  this.ts_parse("2006/01/02")  # 'this' refers to original this.date
)

# Or: Explicit error binding
root.parsed = this.date.ts_parse("2006-01-02").catch(err, val) {
  val.ts_parse("2006/01/02")  # 'val' is the value that failed (this.date)
}
```

### Implementation Considerations
- Parser changes to enforce explicit context binding
- Documentation updates to clarify context rules
- Linter rules to warn on implicit context usage
- Consider deprecation path for implicit match context

### Backward Compatibility
⚠️ **Breaking change** if enforced immediately
- Phase 1: Add `as` syntax as optional
- Phase 2: Lint warnings for implicit context
- Phase 3: Make explicit context required (v2.0)

---

## Solution 3: Unify Execution Models

### Problem Reference
Issue #3: Dual Execution Models (Mapping vs Mutation)

### Proposed Solution
Unify around immutable-by-default model with explicit mutation markers.

### Option A: Deprecate Mutation Processor

**Approach:** Remove mutation processor entirely, optimize mapping processor performance.

**Pros:**
- Single mental model
- Eliminates order-dependent bugs
- Simpler to reason about

**Cons:**
- Performance regression for in-place modifications
- Breaking change for existing mutation users

### Option B: Explicit Mutation Syntax

**Approach:** Keep both processors but require explicit mutation markers in code.

```bloblang
# In mutation processor, require explicit markers
root = this  # Error: ambiguous, which model?

# Explicit copy (mapping semantics)
root = this.clone()
root.invitees = this.invitees.filter(i -> i.mood >= 0.5)
root.rejected = this.invitees.filter(i -> i.mood < 0.5)  # Both see original

# Explicit mutation (mutation semantics)
root.invitees = mut this.invitees.filter(i -> i.mood >= 0.5)
root.rejected = mut this.invitees.filter(i -> i.mood < 0.5)  # Sees mutated

# Or use different operator
root.invitees := this.invitees.filter(i -> i.mood >= 0.5)  # := means mutate
root.rejected = this.invitees.filter(i -> i.mood < 0.5)    # = means read
```

### Option C: Single Processor with Mode Declaration

**Approach:** One processor with explicit mode declaration at top of script.

```bloblang
mode: immutable  # or: mode: mutable

# Behavior determined by mode declaration
root.invitees = this.invitees.filter(i -> i.mood >= 0.5)
root.rejected = this.invitees.filter(i -> i.mood < 0.5)
```

### Recommendation
**Option C** - Single processor with explicit mode declaration
- Makes execution model visible in the code
- Allows linting to catch order-dependent bugs
- Performance benefits of mutation when needed
- Clear migration path (add mode declarations to existing scripts)

### Implementation Considerations
- Performance optimization for immutable model (copy-on-write, structural sharing)
- Linter to detect order-dependent patterns in mutable mode
- Clear error messages when mode mismatch occurs
- Benchmark suite to ensure performance parity

### Backward Compatibility
⚠️ **Requires migration**
- Default to immutable mode for new scripts
- Mutation processor automatically adds `mode: mutable` during migration
- Provide migration tool to analyze and update existing scripts

---

## Solution 4: Smart Error Handling Defaults

### Problem Reference
Issue #4: Verbose Error Handling Requirements

### Proposed Solution
Introduce null-safe operators and safe-by-default variants of common operations.

### Syntax Proposal

```bloblang
# Option A: Null-safe navigation operator (?.)
root.name = this.user?.name              # Returns null if user is null
root.email = this.user?.profile?.email   # Chains safely
root.first = this.items?[0]              # Safe array access

# Option B: Default value operator (?:)
root.name = this.user.name ?: "anonymous"     # Shorthand for .or()
root.count = this.items.length() ?: 0         # Shorthand for .catch()

# Option C: Safe method variants (try_ prefix)
root.data = this.payload.try_parse_json()     # Returns null on failure
root.count = this.items.try_length()          # Returns null on failure
root.first = this.items.try_at(0)             # Returns null on invalid index

# Option D: Optional type system
root.name = this.user.name?                   # Explicitly optional type
# Compiler ensures null handling before using value
root.upper = root.name.unwrap_or("default").uppercase()
```

### Recommendation
**Combination of A and B**
- `?.` for null-safe navigation (like TypeScript, C#, Swift)
- `?:` as shorthand for common fallback pattern (like Kotlin's Elvis operator)
- Keep explicit `.catch()` and `.or()` for complex cases

### Implementation Considerations
- Parser changes for new operators
- Ensure operator precedence is intuitive
- Performance impact of additional null checks
- Clear semantics for chaining operators

### Backward Compatibility
✅ **Fully backward compatible** - Adds new operators, existing code unchanged

### Examples

```bloblang
# Before: Verbose defensive code
root.display = this.user.name.or("anonymous").uppercase()
root.count = this.items.length().catch(0)
root.first = this.items[0].catch(null)

# After: Concise with operators
root.display = (this.user?.name ?: "anonymous").uppercase()
root.count = this.items.length() ?: 0
root.first = this.items?[0]

# Chaining safely
root.city = this.user?.address?.city ?: "Unknown"

# Complex fallback still uses catch
let date_str = this.date
root.parsed = $date_str.ts_parse("2006-01-02").catch(
  $date_str.ts_parse("2006/01/02")
) ?: null
```

---

## Solution 5: Enhanced Lambda Expressions

### Problem Reference
Issue #5: Limited Lambda Expressions

### Proposed Solution
Support multi-parameter lambdas and multi-statement lambda bodies.

### Syntax Proposal

```bloblang
# Multi-parameter lambdas
this.items.reduce((acc, item) -> acc + item.value, 0)
this.pairs.map_each((key, val) -> {"k": key, "v": val})

# Multi-statement lambda bodies with braces
this.items.map_each(item -> {
  let processed = item.value.uppercase()
  let tagged = $processed + "_" + item.id
  {"result": $tagged, "original": item}
})

# Named function definitions (syntactic sugar for maps)
fn format_user(user) -> {
  "id": user.id,
  "name": user.first_name + " " + user.last_name,
  "email": user.contact.email
}

root.customers = this.customers.map_each(format_user)
```

### Implementation Considerations
- Parser changes for parameter lists and block syntax
- Scope management for lambda-local variables
- Performance implications of closures
- Ensure lambda parameters shadow outer scope appropriately

### Backward Compatibility
✅ **Fully backward compatible** - Extends lambda syntax, existing single-param lambdas unchanged

### Examples

```bloblang
# Tuple-like operations
this.items
  .enumerate()
  .filter((idx, val) -> idx < 10 && val.score > 50)
  .map_each((idx, val) -> val)

# Complex transformations
this.records.map_each(record -> {
  let base_score = record.value * 2
  let bonus = if record.premium { 10 } else { 0 }
  let total = $base_score + $bonus
  {
    "id": record.id,
    "score": $total,
    "tier": if $total > 100 { "gold" } else { "silver" }
  }
})

# Reusable function definitions
fn calculate_discount(price, customer_tier) -> {
  let rate = match customer_tier {
    "gold" => 0.20
    "silver" => 0.10
    _ => 0.05
  }
  price * (1 - $rate)
}

root.items = this.items.map_each(item -> {
  "price": calculate_discount(item.price, this.customer.tier)
})
```

---

## Solution 6: String Interpolation

### Problem Reference
Issue #6: No String Interpolation

### Proposed Solution
Add string interpolation syntax with automatic type coercion.

### Syntax Proposal

```bloblang
# Option A: ${} syntax (like JavaScript, Kotlin, many others)
root.message = "User ${this.name} has ${this.count} items"
root.path = "/users/${this.id}/profile"

# Option B: #{} syntax (like Ruby, to avoid conflict if $ used elsewhere)
root.message = "User #{this.name} has #{this.count} items"

# Option C: %{} syntax (like Elixir)
root.message = "User %{this.name} has %{this.count} items"

# With expressions inside
root.summary = "Total: ${this.items.length()} items worth $${this.total.round(2)}"

# With format specifiers (optional enhancement)
root.formatted = "Price: ${this.price:.2f} on ${this.date:%Y-%m-%d}"
```

### Recommendation
**Option A: `${}` syntax**
- Most familiar to developers from other languages
- Variables already use `$` prefix, so consistent
- Clear delimiters that stand out

### Implementation Considerations
- Parser changes to recognize interpolation syntax
- Automatic `.string()` coercion for non-string values
- Escaping mechanism for literal `${` in strings
- Performance of string building vs concatenation

### Backward Compatibility
✅ **Fully backward compatible** - Only affects string literals with `${`, which previously wouldn't have been used

### Examples

```bloblang
# Before
root.message = "User " + this.name + " has " + this.count.string() + " items"
root.log = "[" + this.level + "] " + this.timestamp.ts_format("2006-01-02") + ": " + this.message

# After
root.message = "User ${this.name} has ${this.count} items"
root.log = "[${this.level}] ${this.timestamp.ts_format("2006-01-02")}: ${this.message}"

# Complex expressions
root.summary = "Processed ${this.items.length()} items in ${this.duration}ms (avg: ${(this.duration / this.items.length()).round(2)}ms/item)"

# Escaping
root.literal = "The syntax is \${expression}"  # Produces: "The syntax is ${expression}"

# Multiline strings with interpolation
root.report = """
Order Summary for ${this.customer.name}
Total Items: ${this.items.length()}
Total Cost: $${this.total.round(2)}
Status: ${this.status.uppercase()}
"""
```

---

## Solution 7: Static Analysis and Type Hints

### Problem Reference
Issue #7: Runtime-Only Type Checking

### Proposed Solution
Add optional type annotations and static analysis tooling without making language statically typed.

### Syntax Proposal

```bloblang
# Option A: Type hints in comments (no language changes)
root.count = this.items.length()  # @type: number
root.name = this.user.name        # @type: string?

# Option B: Optional type declarations
root.count: number = this.items.length()
root.name: string? = this.user.name  # ? indicates nullable

# Option C: Type assertions (runtime checks)
root.count = this.items.length() as number
root.name = this.user.name as string?

# Function signatures with types
fn calculate_price(base: number, tax_rate: number) -> number {
  base * (1 + tax_rate)
}
```

### Recommendation
**Phased Approach:**
1. **Phase 1**: Static analyzer that works with existing code (no syntax changes)
2. **Phase 2**: Optional type hint comments for tooling
3. **Phase 3**: Optional type annotations in language (opt-in)

### Tooling Proposal

```bash
# Static analyzer command
rpk connect blobl lint ./mapping.blobl

# Example output
mapping.blobl:5:15: warning: this.count may be null, consider using .or() or null check
mapping.blobl:8:20: error: method .uppercase() requires string, but this.value may be number
mapping.blobl:12:10: suggestion: variable $user_id is declared but never used
```

### Implementation Considerations
- Type inference engine to track types through mappings
- Integration with IDE language servers
- Balance between strictness and flexibility
- Clear opt-in/opt-out mechanism

### Backward Compatibility
✅ **Fully backward compatible** - Tooling optional, type annotations optional

### Examples

```bloblang
# Analyzer detects potential issues
root.total = this.price * this.quantity
# Warning: this.price might be null (detected from data samples)
# Suggestion: root.total = (this.price * this.quantity).catch(null)

# With type hints (for better analysis)
let price: number = this.price.or(0)
let quantity: number = this.quantity.or(1)
root.total: number = $price * $quantity
# No warnings - types proven safe

# Function with type checking
fn format_currency(amount: number, currency: string = "USD") -> string {
  "$${amount.round(2)} ${currency}"
}

root.price = format_currency(this.amount, this.currency)
# Analyzer checks: this.amount is number, this.currency is string
```

---

## Solution 8: Iteration Syntax Sugar

### Problem Reference
Issue #8: No Iteration Constructs

### Proposed Solution
Add familiar iteration syntax that compiles to existing functional constructs or recursive maps.

### Syntax Proposal

```bloblang
# Option A: For expressions (comprehension style)
root.doubled = for item in this.items {
  item * 2
}

root.filtered = for item in this.items if item.score > 50 {
  item.name
}

# Option B: Range-based iteration
root.sequence = for i in range(0, 10) {
  {"index": i, "value": i * 2}
}

# Option C: While expressions (for stateful iteration)
root.result = {
  let mut count = 0
  let mut sum = 0
  while count < this.items.length() {
    sum = sum + this.items[count]
    count = count + 1
  }
  sum
}

# Option D: Fold/reduce syntax
root.total = fold this.items with sum = 0 {
  sum + item.value
}
```

### Recommendation
**Option A + D**: Comprehension syntax for common cases, explicit fold for complex reductions
- Comprehensions compile to `.map_each()` and `.filter()` combinations
- Fold provides readable syntax for reductions
- No mutable state required (purely functional under the hood)

### Implementation Considerations
- Desugar to existing functional operations where possible
- Comprehensions must be pure (no side effects)
- Consider generator expressions for lazy evaluation
- Performance optimization for large collections

### Backward Compatibility
✅ **Fully backward compatible** - Adds new syntax, doesn't change existing

### Examples

```bloblang
# Before: Using map_each and filter
root.processed = this.items
  .filter(item -> item.active)
  .map_each(item -> item.value * 2)

# After: Using comprehension
root.processed = for item in this.items if item.active {
  item.value * 2
}

# Nested comprehensions
root.matrix = for row in this.data {
  for col in row if col > 0 {
    col * 2
  }
}

# With fold for accumulation
root.stats = fold this.items with acc = {"sum": 0, "count": 0} {
  {
    "sum": acc.sum + item.value,
    "count": acc.count + 1
  }
}

# Traditional syntax still works
root.processed = this.items
  .filter(item -> item.active)
  .map_each(item -> item.value * 2)
```

---

## Solution 9: Enhanced Module System

### Problem Reference
Issue #9: Limited Module System

### Proposed Solution
Add namespace management and selective imports.

### Syntax Proposal

```bloblang
# Selective imports
import { extract_user, format_date } from "./user_utils.blobl"
import { calculate_tax } from "./financial.blobl"

# Namespace imports
import * as user_utils from "./user_utils.blobl"
import * as date_utils from "./date_utils.blobl"

# Usage with namespace
root.customer = this.customer_data.apply(user_utils.extract_user)
root.formatted = date_utils.format_iso8601(this.timestamp)

# Export declarations in imported file
export map extract_user {
  root.id = this.user_id
  root.name = this.full_name
}

export fn format_iso8601(ts) -> {
  ts.ts_format("2006-01-02T15:04:05Z07:00")
}

# Private maps (not exported)
map internal_helper {
  # Only available within this file
}

# Re-exports
export { extract_user, format_date } from "./user_utils.blobl"
```

### Implementation Considerations
- Module resolution strategy (relative, absolute, package paths)
- Circular dependency detection
- Module caching and compilation
- Namespace collision detection
- Consider package management integration

### Backward Compatibility
⚠️ **Minor breaking change**
- Current `import` loads everything into global scope
- Migration: `import "./file.blobl"` becomes `import * from "./file.blobl"`
- Provide migration tool to update import statements

### Examples

```bloblang
# user_transforms.blobl
export fn format_full_name(user) -> {
  "${user.first_name} ${user.last_name}"
}

export fn format_email_display(user) -> {
  "${format_full_name(user)} <${user.email}>"
}

map internal_normalize {
  # Private helper, not exported
}

# main.blobl
import { format_email_display } from "./user_transforms.blobl"
import * as dates from "./date_utils.blobl"

root.users = this.users.map_each(user -> {
  "display": format_email_display(user),
  "joined": dates.format_iso8601(user.created_at)
})
```

---

## Solution 10: Destructuring Assignment

### Problem Reference
Issue #10: No Destructuring

### Proposed Solution
Add destructuring syntax for objects and arrays in variable declarations and lambda parameters.

### Syntax Proposal

```bloblang
# Object destructuring
let {id, name, email} = this.user
root.summary = "User ${name} (${id})"

# Nested destructuring
let {user: {id, profile: {email}}} = this.data
root.contact = email

# Array destructuring
let [first, second, ...rest] = this.items
root.top_two = [first, second]
root.others = rest

# Destructuring in lambda parameters
this.users.map_each(({id, name}) -> {
  "user_id": id,
  "display_name": name
})

# With defaults
let {name = "anonymous", age = 0} = this.user
root.info = "${name} is ${age} years old"

# Rest operator
let {id, ...other_fields} = this.user
root.id = id
root.metadata = other_fields
```

### Implementation Considerations
- Parser changes for destructuring patterns
- Scope management for destructured variables
- Error handling for missing fields
- Performance impact of destructuring

### Backward Compatibility
✅ **Fully backward compatible** - Adds new syntax to let statements

### Examples

```bloblang
# Before: Verbose field extraction
let user_id = this.user.id
let user_name = this.user.name
let user_email = this.user.email

root.summary = {
  "id": user_id,
  "name": user_name,
  "email": user_email
}

# After: Concise destructuring
let {id, name, email} = this.user
root.summary = {id, name, email}  # Shorthand property names

# Complex nested structures
let {
  order: {id: order_id, total},
  customer: {name: customer_name, tier},
  items: [first_item, ...other_items]
} = this.transaction

root.summary = {
  "order_id": order_id,
  "customer": customer_name,
  "total": total,
  "first_item": first_item.name,
  "remaining_count": other_items.length()
}

# In lambda transformations
root.formatted_users = this.users.map_each(({id, first_name, last_name, email}) -> {
  "id": id,
  "full_name": "${first_name} ${last_name}",
  "contact": email
})
```

---

## Solution 11: Improved Documentation and Discovery

### Problem Reference
Issue #11: Discovery Complexity

### Proposed Solution
Multi-pronged approach to improve discoverability.

### Proposal Components

#### 1. Interactive Documentation Browser
```bash
# Launch interactive docs
rpk connect blobl docs

# Search functions
rpk connect blobl docs search "timestamp"

# Show function details with examples
rpk connect blobl docs show ts_parse
```

#### 2. IDE Integration
- Language Server Protocol (LSP) implementation
- Autocomplete for functions, methods, and field paths
- Inline documentation on hover
- Jump to definition for maps and imports

#### 3. Categorized Cheat Sheet
```bash
# Generate cheat sheet by category
rpk connect blobl cheatsheet --category strings
rpk connect blobl cheatsheet --category arrays
rpk connect blobl cheatsheet --all > bloblang_reference.md
```

#### 4. "Did You Mean?" Suggestions
```bloblang
root.upper = this.text.to_uppercase()
# Error: method 'to_uppercase' not found. Did you mean 'uppercase'?

root.uid = generate_uuid()
# Error: function 'generate_uuid' not found. Did you mean 'uuid_v4'?
```

#### 5. Common Recipes Documentation
```bash
# Built-in cookbook
rpk connect blobl recipe list
rpk connect blobl recipe show csv-to-json
rpk connect blobl recipe show timestamp-conversion
```

### Implementation Considerations
- Documentation generator improvements
- LSP server development and maintenance
- Search indexing for function/method discovery
- Community contribution framework for recipes

### Backward Compatibility
✅ **N/A** - Tooling and documentation improvements only

---

## Solution 12: Enhanced Debugging Support

### Problem Reference
Issue #12: Limited Debugging

### Proposed Solution
Add debugging tools and better error reporting.

### Proposal Components

#### 1. Debug Mode Execution
```bash
# Run with debug output showing intermediate values
rpk connect blobl run --debug ./mapping.blobl < input.json

# Example output:
# [DEBUG] Line 3: let user_id = "user_123"
# [DEBUG] Line 4: let name = "John Doe"
# [DEBUG] Line 5: root.display = "John Doe (user_123)"
```

#### 2. Trace Points
```bloblang
# Special debug function that logs values without affecting execution
root.processed = this.items
  .filter(item -> {
    debug("Filtering item", item)  # Logs to stderr in debug mode
    item.score > 50
  })
  .map_each(item -> {
    debug("Processing item", item)
    item.value * 2
  })
```

#### 3. Interactive Debugger
```bash
# Start interactive REPL
rpk connect blobl repl < input.json

> this.user.name
"John Doe"

> this.items.length()
5

> this.items.filter(i -> i.score > 50)
[{"id": 1, "score": 75}, {"id": 3, "score": 90}]
```

#### 4. Better Error Messages
```bloblang
# Current error:
# mapping error: failed to execute mapping

# Improved error:
# Error at line 5, column 15:
#   root.total = this.price * this.quantity
#                ^^^^^^^^^^^
# Type error: cannot multiply null by number
#   this.price is null
#   this.quantity is 5
# Suggestion: use .or() to provide default: this.price.or(0) * this.quantity
```

#### 5. Dry-Run with Verbose Output
```yaml
# In config
pipeline:
  processors:
    - mapping:
        mapping: |
          root.value = this.input * 2
        debug: true  # Logs all intermediate values
```

### Implementation Considerations
- Performance impact of debug mode
- Structured logging for debug output
- REPL implementation with state management
- Error context preservation through execution

### Backward Compatibility
✅ **Fully backward compatible** - Tooling additions only

---

## Solution 13: Strict Mode Option

### Problem Reference
Issue #13: Implicit Field Creation Behavior

### Proposed Solution
Add optional strict mode that requires explicit field declarations or validation.

### Syntax Proposal

```bloblang
# Option A: Mode declaration
mode: strict

# In strict mode, unknown fields are errors
root.user.emial = this.email
# Error: field 'emial' not declared. Did you mean 'email'?

# Option B: Schema declaration
schema {
  user: {
    id: string
    name: string
    email: string  # Only these fields allowed
  }
}

root.user.id = this.id
root.user.name = this.name
root.user.email = this.email
root.user.emial = this.email  # Error: field 'emial' not in schema

# Option C: Linter rules (no language changes)
# .bloblang-lint.yaml
rules:
  - detect-typos: true
  - max-field-depth: 5
  - require-explicit-fields: false
```

### Recommendation
**Option C initially, Option B long-term**
- Start with linter to detect suspicious patterns
- Add optional schema declarations for validation
- Keep permissive behavior as default

### Implementation Considerations
- Typo detection using edit distance algorithms
- Schema format and validation
- IDE integration for real-time feedback
- Balance between strictness and flexibility

### Backward Compatibility
✅ **Fully backward compatible** - Opt-in strict mode

### Examples

```bloblang
# With schema validation
schema {
  output: {
    user_id: string
    user_name: string
    user_email: string
  }
}

root.user_id = this.id
root.user_name = this.name
root.user_emial = this.email  # Error caught at lint/compile time

# Linter suggestions
root.usr.name = this.user.name
# Warning: suspicious field name 'usr' at depth 1. Did you mean 'user'?

root.metadata.extra.data.nested.deep.value = this.value
# Warning: deeply nested field (depth 6) - consider flattening structure
```

---

## Implementation Priority Recommendations

### Phase 1: High-Impact, Low-Risk (v4.6)
1. **Solution 6**: String Interpolation - High demand, fully backward compatible
2. **Solution 4**: Null-Safe Operators - Improves ergonomics significantly, backward compatible
3. **Solution 11**: Documentation Improvements - Pure tooling, immediate benefit
4. **Solution 12**: Debug Tools - Pure tooling, aids development

### Phase 2: Medium-Impact, Backward Compatible (v4.7)
1. **Solution 1**: Block-Scoped Variables - High demand, backward compatible
2. **Solution 10**: Destructuring - Nice ergonomics improvement
3. **Solution 7**: Static Analysis - Tooling first, then optional annotations
4. **Solution 8**: Iteration Syntax - Syntactic sugar for existing patterns

### Phase 3: High-Impact, Breaking Changes (v5.0)
1. **Solution 3**: Unify Execution Models - Requires careful migration
2. **Solution 2**: Explicit Context Management - Breaking but essential for reliability
3. **Solution 9**: Enhanced Module System - Breaking but necessary for scale

### Phase 4: Lower Priority Enhancements (v5.x)
1. **Solution 5**: Enhanced Lambdas - Nice to have, adds complexity
2. **Solution 13**: Strict Mode - Opt-in safety feature

---

## Next Steps

1. **Review and feedback** on these proposals from core team and community
2. **Prototype** Phase 1 solutions in experimental branch
3. **RFC process** for breaking changes (Phase 3)
4. **Create feature flags** for gradual rollout
5. **Update test suite** to cover new syntax
6. **Documentation updates** alongside implementation
7. **Migration guides** for breaking changes

---

## Open Questions

1. Should we version the language syntax (Bloblang v1 vs v2)?
2. How do we handle gradual migration for breaking changes?
3. What level of backward compatibility is required? (1 year? 2 years?)
4. Should we maintain a compatibility mode indefinitely?
5. How do these changes affect plugin authors and external integrations?
6. What's the testing strategy for each proposal?
7. Performance benchmarks - what's acceptable overhead for new features?

---

**End of Proposals Document**
