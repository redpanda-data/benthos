# Bloblang Language Technical Specification

**Version:** 1.0
**Date:** 2026-02-10

## 1. Overview

Bloblang (blobl) is a domain-specific mapping language for transforming structured and unstructured data within stream processing pipelines. It operates on immutable input documents to produce new output documents through declarative assignment statements.

## 2. Lexical Structure

### 2.1 Tokens

- **Identifiers**: `[a-zA-Z_][a-zA-Z0-9_]*`
- **Keywords**: `root`, `this`, `let`, `meta`, `if`, `else`, `match`, `map`, `deleted`, `import`, `from`
- **Operators**: `.`, `=`, `|`, `+`, `-`, `*`, `/`, `%`, `!`, `>`, `>=`, `==`, `<`, `<=`, `&&`, `||`, `=>`, `->`
- **Delimiters**: `(`, `)`, `{`, `}`, `[`, `]`, `,`, `:`
- **Literals**: Numbers, strings, booleans, null, arrays, objects
- **Comments**: `#` to end-of-line

### 2.2 Identifiers

Field names follow dot-separated path notation. Special characters (spaces, dots, symbols) require double-quote escaping:
```
root.user.name          # Standard identifier
root."foo.bar".baz      # Quoted identifier with dot
root."field with spaces" # Quoted identifier with spaces
```

### 2.3 Literals

**Numbers**: Integer or floating-point decimal notation.
```
42
3.14
-10
```

**Strings**: Single-line (double quotes) or multi-line (triple quotes).
```
"hello world"
"""
multi-line
string
"""
```

**Booleans**: `true`, `false`

**Null**: `null`

**Arrays**: Comma-separated values in square brackets. May contain dynamic query expressions.
```
[1, 2, 3]
["a", this.field, uuid_v4()]
```

**Objects**: Comma-separated key-value pairs in curly braces. Keys are strings; values may be dynamic.
```
{"name": "value", "count": 42}
{"id": this.id, "timestamp": now()}
```

## 3. Type System

Bloblang is dynamically typed with the following runtime types:
- **Number**: 64-bit integer or floating-point
- **String**: UTF-8 encoded character sequence
- **Boolean**: `true` or `false`
- **Null**: Represents absence of value
- **Bytes**: Raw byte sequence (via `content()` function)
- **Array**: Ordered collection of heterogeneous values
- **Object**: Unordered collection of key-value pairs

Type introspection via `.type()` method returns: `"number"`, `"string"`, `"bool"`, `"null"`, `"bytes"`, `"array"`, `"object"`.

## 4. Expressions

### 4.1 Path Expressions

Navigate nested structures using dot notation:
```
this.user.profile.email
root.output.data.id
```

Paths may reference:
- **Input document**: `this.field`
- **Output document**: `root.field`
- **Variables**: `$variable_name`
- **Metadata**: `@metadata_key` or `metadata("key")`

### 4.2 Boolean Operators

- `!` - logical NOT
- `>`, `>=`, `==`, `<`, `<=` - comparison (value and type equality)
- `&&` - logical AND
- `||` - logical OR

### 4.3 Arithmetic Operators

- `+` - addition or string concatenation
- `-` - subtraction
- `*` - multiplication
- `/` - division
- `%` - modulo

### 4.4 Coalescing Operator

Pipe operator `|` selects the first non-null value from alternative paths:
```
this.article.body | this.comment.text | "default"
this.data.(primary_id | secondary_id | backup_id)
```

**Semantic**: Returns first successful path evaluation. Differs from `.catch()` which handles operation failures.

### 4.5 Functions

Functions generate or retrieve values without a target:
```
uuid_v4()                    # Generate UUID
now()                        # Current timestamp
hostname()                   # System hostname
content()                    # Raw message bytes
env("VAR_NAME")              # Environment variable
random_int(seed, min, max)   # Random integer
deleted()                    # Deletion marker
```

**Syntax**: `function_name()` or `function_name(args...)`

**Arguments**: Positional or named.
```
random_int(timestamp_unix_nano(), 1, 100)
random_int(seed: timestamp_unix_nano(), min: 1, max: 100)
```

### 4.6 Methods

Methods transform target values and support chaining:
```
this.text.uppercase()
this.data.parse_json()
this.items.filter(x -> x.score > 50)
this.name.trim().lowercase().replace_all("_", "-")
```

**Syntax**: `target.method_name()` or `target.method_name(args...)`

**Common Categories**:
- **String**: `.uppercase()`, `.lowercase()`, `.trim()`, `.replace_all()`, `.split()`, `.contains()`
- **Number**: `.floor()`, `.ceil()`, `.round()`, `.abs()`
- **Array**: `.filter()`, `.map_each()`, `.sort()`, `.sort_by()`, `.length()`, `.join()`
- **Object**: `.keys()`, `.values()`, `.map_each()`, `.without()`
- **Parsing**: `.parse_json()`, `.parse_csv()`, `.parse_xml()`, `.parse_yaml()`
- **Encoding**: `.encode_base64()`, `.decode_base64()`, `.format_json()`, `.hash()`
- **Timestamp**: `.ts_parse()`, `.ts_format()`, `.ts_unix()`
- **Type**: `.string()`, `.number()`, `.bool()`, `.type()`, `.not_null()`, `.not_empty()`
- **Error Handling**: `.catch()`, `.or()`

### 4.7 Lambda Expressions

Anonymous functions for higher-order methods (`.filter()`, `.map_each()`, `.sort_by()`):
```
this.items.filter(i -> i.score > 50)
this.items.map_each(i -> i.name.uppercase())
this.items.sort_by(i -> i.timestamp)
```

**Syntax**: `parameter -> expression`

**Named Context**: `parameter_name -> parameter_name.field` for clarity.

**Parenthesized Context**: `this.foo.(x -> x.bar + x.baz)` changes context scope.

## 5. Statements

### 5.1 Assignment Statement

Assigns expression result to output document path:
```
root.field = expression
root = this                    # Copy entire input
root.user.id = this.id         # Nested field creation
root."special.field" = value   # Quoted field name
```

**Semantics**: Creates intermediate objects as needed. Assignments to `root` build new document.

### 5.2 Metadata Assignment

Assigns to message metadata using `meta` keyword:
```
meta output_key = this.id
meta content_type = "application/json"
@kafka_topic = "new-topic"
```

### 5.3 Variable Declaration

Declares reusable values with `let`:
```
let user_id = this.user.id
let processed = $user_id.string().uppercase()
```

**Scope**: Variables are top-level only; cannot be declared inside `if`, `match`, or lambda expressions.

**Reference**: Use `$` prefix: `$variable_name`

### 5.4 Deletion

Removes fields using `deleted()` function:
```
root.password = deleted()          # Remove field
root = if this.spam { deleted() }  # Filter message
```

**Semantics**: Assigning `deleted()` to a field excludes it from output. Assigning to `root` filters entire message.

## 6. Control Flow

### 6.1 If Expression

Conditional expression returning a value:
```
root.category = if this.score >= 80 {
  "high"
} else if this.score >= 50 {
  "medium"
} else {
  "low"
}
```

**Syntax**: `if condition { true_branch } [else if condition { branch }]* [else { false_branch }]`

**Semantics**: Returns value of executed branch. Omitting `else` with no match skips assignment.

### 6.2 If Statement

Conditional execution of multiple assignments without return value:
```
if this.type == "user" {
  root.role = "member"
  root.permissions = ["read"]
}
```

### 6.3 Match Expression

Pattern matching with multiple cases:
```
root.sound = match this.animal {
  "cat" => "meow"
  "dog" => "woof"
  this.animal.contains("bird") => "chirp"
  _ => "unknown"
}
```

**Syntax**: `match [expression] { case => result [, case => result]* }`

**Cases**: Boolean expressions, literal comparisons, or `_` (catch-all).

**Context**: When expression specified (`match this.field`), context inside cases shifts to that expression.

**Semantics**: Evaluates cases sequentially; returns first matching result.

### 6.4 Match Statement

Pattern matching executing multiple assignments:
```
match {
  this.type() == "object" => {
    root = this.map_each(item -> item.value.apply("transform"))
  }
  this.type() == "array" => {
    root = this.map_each(ele -> ele.apply("transform"))
  }
  _ => {
    root = this
  }
}
```

## 7. Maps (Named Mappings)

Reusable transformation definitions:
```
map extract_user {
  root.id = this.user_id
  root.name = this.full_name
  root.email = this.contact.email
}

root.customer = this.customer_data.apply("extract_user")
```

**Syntax**: `map name { statements }`

**Invocation**: `.apply("map_name")` method

**Parameters**: Single context parameter accepted; pass multiple values via object literal:
```
{"value": this.a, "pattern": "[%v]"}.apply("formatter")
```

**Recursion**: Maps may recursively invoke themselves:
```
map walk_tree {
  root = match {
    this.type() == "object" => this.map_each(item -> item.value.apply("walk_tree"))
    this.type() == "array" => this.map_each(ele -> ele.apply("walk_tree"))
    _ => this
  }
}
```

## 8. Imports

Import mappings from external files:
```
import "./transformations.blobl"
```

**Syntax**: `import "path"`

**Path Resolution**: Absolute paths or relative to execution directory.

**Execution from File**: `from "<path>"` executes entire mapping from file.

## 9. Error Handling

### 9.1 Catch Method

Provides fallback value on operation failure:
```
root.count = this.items.length().catch(0)
root.parsed = this.data.parse_json().catch({})
root.value = (this.price * this.quantity).catch(null)
```

**Semantics**: On error anywhere in method chain, returns fallback value and suppresses error propagation.

### 9.2 Or Method

Provides fallback for `null` values:
```
root.name = this.user.name.or("anonymous")
root.id = this.primary_id.or(this.secondary_id)
```

**Semantics**: Returns fallback if target is `null`; distinct from `.catch()` which handles errors.

### 9.3 Throw Function

Manually raises errors with custom messages:
```
root.value = if this.required_field == null {
  throw("Missing required field")
} else {
  this.required_field
}
```

### 9.4 Validation Methods

Type validation methods throw errors on failure:
```
root.count = this.count.number()      # Error if not number
root.name = this.name.not_null()      # Error if null
root.items = this.items.not_empty()   # Error if empty
```

## 10. Metadata

Access message metadata using `@` prefix or `metadata()` function:
```
root.topic = @kafka_topic
root.partition = @kafka_partition
root.key = metadata("kafka_key")
```

Modify metadata using `meta` keyword:
```
meta output_topic = "processed"
meta kafka_key = this.id
```

Delete metadata:
```
meta kafka_key = deleted()
```

## 11. Execution Model

### 11.1 Mapping Processor (Immutable)

Creates entirely new output document. Input document (`this`) remains immutable throughout execution:
```
root.id = this.id
root.invitees = this.invitees.filter(i -> i.mood >= 0.5)
root.rejected = this.invitees.filter(i -> i.mood < 0.5)  # Original still accessible
```

**Use Case**: Output shape significantly differs from input.

### 11.2 Mutation Processor (Mutable)

Directly modifies input document. Input document changes during execution:
```
root.rejected = this.invitees.filter(i -> i.mood < 0.5)  # Copy before mutation
root.invitees = this.invitees.filter(i -> i.mood >= 0.5) # Mutates original
```

**Use Case**: Output shape similar to input; avoids data copying overhead.

**Caution**: Assignment order matters; later assignments see mutated state.

### 11.3 Evaluation Order

Assignments execute sequentially in source order. Variables must be declared before use.

## 12. Context and Scoping

### 12.1 Root Context

`root` refers to the output document being constructed. Accessible throughout mapping:
```
root.field = value
root = this.data
```

### 12.2 This Context

`this` refers to current query context:
- **Top-level**: Input document
- **Inside match with expression**: Matched expression value
- **Inside lambda**: Lambda parameter
- **Inside parenthesized context**: Redefined context

### 12.3 Variable Scope

Variables declared with `let` have top-level scope:
```
let user_id = this.user.id
let name = this.user.name

root.id = $user_id
root.name = $name
```

**Restriction**: Cannot declare variables inside `if`, `match`, or lambda expressions.

### 12.4 Context Manipulation

Parenthesized expressions change context scope:
```
this.foo.(this.bar + this.baz)           # `this` refers to `this.foo`
this.foo.(x -> x.bar + x.baz)            # Named context `x`
```

## 13. Special Features

### 13.1 Non-Structured Data

`content()` function retrieves raw message bytes for unstructured data processing:
```
root = content().string().uppercase()
```

Assigning primitive values to `root` produces non-structured output:
```
root = "plain text output"
root = 42
```

### 13.2 Dynamic Field Names

Computed field names in objects:
```
root = {
  this.key_field: this.value_field
}
```

### 13.3 Conditional Literals

If expressions and `deleted()` within array and object literals:
```
root = {
  "id": this.id,
  "name": if this.name != null { this.name },
  "age": if this.age > 0 { this.age } else { deleted() }
}
```

**Semantics**: Omitted branches skip field creation; `deleted()` removes field from literal.

### 13.4 Message Filtering

Assigning `deleted()` to `root` filters entire message from pipeline:
```
root = if this.spam { deleted() }
```

### 13.5 Command-Line Execution

`rpk connect blobl` subcommand executes Bloblang scripts directly, treating each input line as separate JSON document.

## 14. Common Patterns

### 14.1 Copy-and-Modify

```
root = this
root.password = deleted()
root.updated_at = now()
```

### 14.2 Null-Safe Access

```
root.name = this.user.name.or("anonymous")
root.id = this.(primary_id | secondary_id | "default")
```

### 14.3 Error-Safe Parsing

```
let date_str = this.date
root.parsed = $date_str.ts_parse("2006-01-02").catch(
  $date_str.ts_parse("2006/01/02")
).catch(null)
```

### 14.4 Array Transformation Pipeline

```
root.results = this.items
  .filter(i -> i.active)
  .map_each(i -> i.name.uppercase())
  .sort()
  .join(", ")
```

### 14.5 Recursive Tree Walking

```
map walk {
  root = match {
    this.type() == "object" => this.map_each(v -> v.apply("walk"))
    this.type() == "array" => this.map_each(e -> e.apply("walk"))
    this.type() == "string" => this.uppercase()
    _ => this
  }
}
root = this.apply("walk")
```

### 14.6 Message Expansion

```
let doc_root = this.without("items")
root = this.items.map_each(item -> $doc_root.merge(item))
```

**Semantics**: Converts single message into array; downstream processors expand into multiple messages.

## 15. Grammar Summary

```
program         := statement*
statement       := assignment | meta_assign | var_decl | map_decl | import_stmt
assignment      := path '=' expression
meta_assign     := 'meta' identifier '=' expression
var_decl        := 'let' identifier '=' expression
map_decl        := 'map' identifier '{' statement* '}'
import_stmt     := 'import' string_literal

expression      := literal | path | function_call | method_chain |
                   if_expr | match_expr | binary_expr | unary_expr |
                   lambda_expr | paren_expr

path            := ('root' | 'this' | var_ref | meta_ref) ('.' field_access)*
field_access    := identifier | quoted_string | '[' expression ']'
var_ref         := '$' identifier
meta_ref        := '@' identifier | 'metadata' '(' string_literal ')'

function_call   := identifier '(' [arg_list] ')'
method_chain    := expression ('.' identifier '(' [arg_list] ')')+

if_expr         := 'if' expression '{' (expression | statement*) '}'
                   ('else' 'if' expression '{' (expression | statement*) '}')*
                   ('else' '{' (expression | statement*) '}')?

match_expr      := 'match' [expression] '{' match_case (',' match_case)* '}'
match_case      := (expression | '_') '=>' (expression | '{' statement* '}')

binary_expr     := expression binary_op expression
binary_op       := '+' | '-' | '*' | '/' | '%' | '|' |
                   '==' | '!=' | '>' | '>=' | '<' | '<=' | '&&' | '||'
unary_expr      := unary_op expression
unary_op        := '!' | '-'

lambda_expr     := identifier '->' expression

literal         := number | string | boolean | null | array | object
array           := '[' [expression (',' expression)*] ']'
object          := '{' [key_value (',' key_value)*] '}'
key_value       := (identifier | string) ':' expression

arg_list        := positional_args | named_args
positional_args := expression (',' expression)*
named_args      := identifier ':' expression (',' identifier ':' expression)*
```

## 16. Type Coercion Rules

- String concatenation: `+` operator converts operands to strings when either operand is string
- Numeric operations: `+`, `-`, `*`, `/`, `%` require numeric operands or result in mapping error
- Boolean operations: `&&`, `||` require boolean operands or result in mapping error
- Comparisons: `==`, `!=` perform type-sensitive equality; `>`, `>=`, `<`, `<=` require comparable types

## 17. Built-in Functions and Methods

Bloblang provides hundreds of built-in functions and methods organized by category:

**Functions**: `uuid_v4`, `ulid`, `ksuid`, `snowflake_id`, `nanoid`, `random_int`, `range`, `now`, `timestamp_*`, `env`, `file`, `hostname`, `content`, `deleted`, `throw`, `count`, `fake_*`, Schema Registry functions

**Methods**: String manipulation, encoding/decoding, hashing, compression, parsing (JSON/CSV/XML/YAML/protobuf), regular expressions, array/object manipulation, timestamp operations, type coercion, GeoIP, JWT, SQL operations

Full reference available via `rpk connect blobl --list-functions` and `rpk connect blobl --list-methods`.

---

**End of Specification**
