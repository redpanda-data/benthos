; Keywords
["if" "else" "match" "as" "map" "import"] @keyword

; Context roots
["input" "output"] @keyword.builtin

; Constants
["true" "false"] @constant.builtin
"null" @constant.builtin

; Discard
"_" @variable.builtin

; Literals
(integer) @number
(float) @number.float
(string) @string
(raw_string) @string
(string_content) @string
(escape_sequence) @string.escape

; Variables ($name)
(variable) @variable

; Map declarations
(map_declaration
  name: (identifier) @function)

; Function calls
(call_expression
  name: (identifier) @function.call)
(call_expression
  name: (qualified_name
    namespace: (identifier) @module
    name: (identifier) @function.call))
"deleted" @function.builtin
"throw" @function.builtin
"void" @function.builtin

; Method calls
(method_call
  method: (_) @function.method)
(null_safe_method_call
  method: (_) @function.method)

; Field access
(field_access
  field: (_) @property)
(null_safe_field_access
  field: (_) @property)

; Parameters
(parameter
  (identifier) @variable.parameter)

; Named arguments
(named_argument
  name: (identifier) @variable.parameter)

; Metadata
(metadata_access) @attribute

; Qualified name namespace
(qualified_name
  namespace: (identifier) @module)

; Operators
["+" "-" "*" "/" "%" "==" "!=" ">" ">=" "<" "<=" "&&" "||" "!"] @operator
["=" "=>" "->"] @operator

; Punctuation
["." "?." "::"] @punctuation.delimiter
["(" ")" "[" "]" "?[" "{" "}"] @punctuation.bracket
["," ":"] @punctuation.delimiter

; Comments
(comment) @comment
