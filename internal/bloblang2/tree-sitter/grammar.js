/// <reference types="tree-sitter-cli/dsl" />
// @ts-check

const PREC = {
  or: 10,
  and: 20,
  equality: 40,
  comparison: 60,
  additive: 80,
  multiplicative: 100,
  unary: 120,
  postfix: 140,
};

module.exports = grammar({
  name: "bloblang2",

  // Two external tokens for newline handling:
  // - _newline: significant newline (statement separator)
  // - _nl_skip: newline consumed as whitespace (inside parens, brackets, etc.)
  //
  // When the parser is in a state where _newline is valid (between statements),
  // the scanner checks for postfix continuation before emitting it.
  // When _newline is not valid (inside expressions, argument lists, etc.),
  // the scanner emits _nl_skip which is in extras and silently consumed.
  externals: ($) => [$._newline, $._nl_skip],

  // Newlines are NOT in extras — they're handled by the external scanner.
  // _nl_skip is in extras so newlines inside (), [], and expression contexts
  // are silently consumed when the parser doesn't expect a statement separator.
  extras: ($) => [/[ \t\r]/, $.comment, $._nl_skip],

  word: ($) => $.identifier,

  conflicts: ($) => [
    // '(' identifier ')' could be lambda params or parenthesized expression
    [$.parameter, $._primary],
    // 'match' '{' — boolean match body or object literal as subject
    [$.match_expression, $.object],
    // $var followed by '.' could be var_assignment path or field_access expression
    [$.var_assignment, $._primary],
  ],

  rules: {
    // Statements separated by newlines. _newline is required between
    // statements (enforcing "one statement per line"). Each _source_item
    // is either a statement or a bare newline, but consecutive statements
    // without an intervening newline will produce a parse error.
    source_file: ($) => repeat($._source_item),

    _source_item: ($) => choice($._top_level_statement, $._newline),

    _top_level_statement: ($) =>
      choice(
        $.assignment,
        $.if_statement,
        $.match_statement,
        $.map_declaration,
        $.import_statement,
      ),

    // --- Assignments ---

    assignment: ($) => seq($.assign_target, "=", $._expression),

    assign_target: ($) =>
      choice(
        seq("output", optional($.metadata_access), repeat($.target_path_segment)),
        seq($.variable, repeat($.target_path_segment)),
      ),

    metadata_access: (_) => "@",

    target_path_segment: ($) =>
      choice(
        seq(".", $._field_name),
        seq(".", $._field_name, "(", optional($.argument_list), ")"),
        seq("[", $._expression, "]"),
      ),

    // --- Map declarations ---

    // map_decl := 'map' id '(' params ')' '{' NL? (var_assignment NL)* expression NL? '}'
    map_declaration: ($) =>
      seq(
        "map",
        field("name", $.identifier),
        "(",
        optional($.parameter_list),
        ")",
        "{",
        optional($._newline),
        $.expr_body,
        optional($._newline),
        "}",
      ),

    parameter_list: ($) => seq($.parameter, repeat(seq(",", $.parameter))),

    parameter: ($) =>
      choice(
        seq($.identifier, "=", $._literal),
        $.identifier,
        "_",
      ),

    // expr_body := (var_assignment NL)* expression
    expr_body: ($) => seq(repeat(seq($.var_assignment, $._newline)), $._expression),

    var_assignment: ($) =>
      seq($.variable, repeat($.target_path_segment), "=", $._expression),

    // --- Imports ---

    import_statement: ($) => seq("import", $.string, "as", $.identifier),

    // --- Expressions ---

    _expression: ($) =>
      choice(
        $._primary,
        $.unary_expression,
        $.binary_expression,
        $.lambda_expression,
        $.if_expression,
        $.match_expression,
        $.field_access,
        $.null_safe_field_access,
        $.method_call,
        $.null_safe_method_call,
        $.index,
        $.null_safe_index,
      ),

    _primary: ($) =>
      choice(
        $.integer,
        $.float,
        $.string,
        $.raw_string,
        $.boolean,
        $.null,
        $.array,
        $.object,
        $.input,
        $.output,
        $.variable,
        $.identifier,
        $.qualified_name,
        $.call_expression,
        $.parenthesized_expression,
      ),

    input: ($) => seq("input", optional($.metadata_access)),
    output: ($) => seq("output", optional($.metadata_access)),

    // --- Postfix operations ---

    field_access: ($) =>
      prec.left(
        PREC.postfix,
        seq(field("receiver", $._expression), ".", field("field", $._field_name)),
      ),

    null_safe_field_access: ($) =>
      prec.left(
        PREC.postfix,
        seq(field("receiver", $._expression), "?.", field("field", $._field_name)),
      ),

    method_call: ($) =>
      prec.left(
        PREC.postfix + 1,
        seq(
          field("receiver", $._expression),
          ".",
          field("method", $._field_name),
          "(",
          optional($.argument_list),
          ")",
        ),
      ),

    null_safe_method_call: ($) =>
      prec.left(
        PREC.postfix + 1,
        seq(
          field("receiver", $._expression),
          "?.",
          field("method", $._field_name),
          "(",
          optional($.argument_list),
          ")",
        ),
      ),

    index: ($) =>
      prec.left(
        PREC.postfix,
        seq(field("receiver", $._expression), "[", field("index", $._expression), "]"),
      ),

    null_safe_index: ($) =>
      prec.left(
        PREC.postfix,
        seq(field("receiver", $._expression), "?[", field("index", $._expression), "]"),
      ),

    // Field names can be identifiers (including keywords) or quoted strings.
    // e.g., input.name, input.map, input."field with spaces"
    _field_name: ($) => choice($._word, $.string),

    // _word includes keywords — used for field/method names after '.' and '?.'
    _word: ($) =>
      choice(
        $.identifier,
        alias("input", $.identifier),
        alias("output", $.identifier),
        alias("if", $.identifier),
        alias("else", $.identifier),
        alias("match", $.identifier),
        alias("as", $.identifier),
        alias("map", $.identifier),
        alias("import", $.identifier),
        alias("true", $.identifier),
        alias("false", $.identifier),
        alias("null", $.identifier),
        alias("deleted", $.identifier),
        alias("throw", $.identifier),
      ),

    // --- Function/method calls ---

    call_expression: ($) =>
      prec(
        PREC.postfix,
        seq(
          field("name", choice($.identifier, $.qualified_name, "deleted", "throw")),
          "(",
          optional($.argument_list),
          ")",
        ),
      ),

    qualified_name: ($) =>
      seq(field("namespace", $.identifier), "::", field("name", $.identifier)),

    argument_list: ($) => choice($.positional_arguments, $.named_arguments),

    positional_arguments: ($) =>
      seq($._expression, repeat(seq(",", $._expression)), optional(",")),

    named_arguments: ($) =>
      seq($.named_argument, repeat(seq(",", $.named_argument)), optional(",")),

    named_argument: ($) =>
      seq(field("name", $.identifier), ":", field("value", $._expression)),

    // --- Unary expressions ---

    unary_expression: ($) =>
      prec(
        PREC.unary,
        seq(field("operator", choice("!", "-")), field("operand", $._expression)),
      ),

    // --- Binary expressions ---

    binary_expression: ($) =>
      choice(
        prec.left(PREC.or, seq(field("left", $._expression), field("operator", "||"), field("right", $._expression))),
        prec.left(PREC.and, seq(field("left", $._expression), field("operator", "&&"), field("right", $._expression))),
        prec.left(PREC.additive, seq(field("left", $._expression), field("operator", choice("+", "-")), field("right", $._expression))),
        prec.left(PREC.multiplicative, seq(field("left", $._expression), field("operator", choice("*", "/", "%")), field("right", $._expression))),
        // Non-associative in spec, left-associative here — semantic analysis rejects chaining.
        prec.left(PREC.equality, seq(field("left", $._expression), field("operator", choice("==", "!=")), field("right", $._expression))),
        prec.left(PREC.comparison, seq(field("left", $._expression), field("operator", choice(">", ">=", "<", "<=")), field("right", $._expression))),
      ),

    // --- Control flow ---

    // if_expr := 'if' expr '{' NL? expr_body NL? '}' (else_if)* (else)?
    if_expression: ($) =>
      prec.right(
        seq(
          "if",
          field("condition", $._expression),
          "{",
          optional($._newline),
          field("consequence", $.expr_body),
          optional($._newline),
          "}",
          repeat($.else_if_clause),
          optional($.else_clause),
        ),
      ),

    // if_stmt := 'if' expr stmt_block (else_if_stmt)* (else_stmt)?
    if_statement: ($) =>
      prec.right(
        seq(
          "if",
          field("condition", $._expression),
          $.statement_block,
          repeat($.else_if_statement_clause),
          optional($.else_statement_clause),
        ),
      ),

    else_if_clause: ($) =>
      seq(
        "else", "if",
        field("condition", $._expression),
        "{",
        optional($._newline),
        field("consequence", $.expr_body),
        optional($._newline),
        "}",
      ),

    else_clause: ($) =>
      seq(
        "else", "{",
        optional($._newline),
        field("alternative", $.expr_body),
        optional($._newline),
        "}",
      ),

    else_if_statement_clause: ($) =>
      seq("else", "if", field("condition", $._expression), $.statement_block),

    else_statement_clause: ($) => seq("else", $.statement_block),

    // stmt_block := '{' (NL | statement)* '}'
    statement_block: ($) =>
      seq("{", repeat(choice($._statement, $._newline)), "}"),

    _statement: ($) =>
      choice($.assignment, $.if_statement, $.match_statement),

    // match_expr := 'match' expr? ('as' id)? '{' match_cases '}'
    // No _newline inside — match cases are comma-separated, so newlines
    // between them are whitespace (consumed by _nl_skip in extras).
    match_expression: ($) =>
      prec.right(
        seq(
          "match",
          optional(seq(
            field("subject", $._expression),
            optional(seq("as", field("binding", $.identifier))),
          )),
          "{",
          optional($.match_cases),
          "}",
        ),
      ),

    match_statement: ($) =>
      prec.right(
        seq(
          "match",
          optional(seq(
            field("subject", $._expression),
            optional(seq("as", field("binding", $.identifier))),
          )),
          "{",
          optional($.match_statement_cases),
          "}",
        ),
      ),

    // Match cases are comma-separated with optional newlines around commas.
    match_cases: ($) => repeat1(seq($.match_case, optional(","))),

    match_case: ($) =>
      seq(
        field("pattern", choice($._expression, "_")),
        "=>",
        field("body", choice(
          $._expression,
          seq("{", optional($._newline), $.expr_body, optional($._newline), "}"),
        )),
      ),

    match_statement_cases: ($) => repeat1(seq($.match_statement_case, optional(","))),

    match_statement_case: ($) =>
      seq(
        field("pattern", choice($._expression, "_")),
        "=>",
        field("body", $.statement_block),
      ),

    // --- Lambda expressions ---

    lambda_expression: ($) =>
      prec.right(
        seq(
          field("parameters", $._lambda_params),
          "->",
          field("body", choice($._expression, $.lambda_block)),
        ),
      ),

    _lambda_params: ($) =>
      choice($.identifier, "_", seq("(", $.parameter_list, ")")),

    // lambda_block := '{' NL? (var_assignment NL)* expression NL? '}'
    lambda_block: ($) =>
      seq("{", optional($._newline), $.expr_body, optional($._newline), "}"),

    // --- Grouped expression ---

    parenthesized_expression: ($) => seq("(", $._expression, ")"),

    // --- Literals ---

    _literal: ($) =>
      choice($.integer, $.float, $.string, $.raw_string, $.boolean, $.null),

    integer: (_) => /[0-9]+/,

    float: (_) => /[0-9]+\.[0-9]+/,

    string: ($) =>
      seq('"', repeat(choice($.escape_sequence, $.string_content)), '"'),

    string_content: (_) => token.immediate(prec(1, /[^"\\\n]+/)),

    escape_sequence: (_) =>
      token.immediate(seq("\\", choice(
        /["\\ntr]/,
        /u[0-9a-fA-F]{4}/,
        /u\{[0-9a-fA-F]{1,6}\}/,
      ))),

    raw_string: (_) => /`[^`]*`/,

    boolean: (_) => choice("true", "false"),

    null: (_) => "null",

    array: ($) =>
      seq("[", optional(seq($._expression, repeat(seq(",", $._expression)), optional(","))), "]"),

    object: ($) =>
      seq(
        "{",
        optional(seq($.object_entry, repeat(seq(",", $.object_entry)), optional(","))),
        "}",
      ),

    object_entry: ($) =>
      seq(field("key", $._expression), ":", field("value", $._expression)),

    // --- Variables ---

    variable: (_) => /\$[a-zA-Z_][a-zA-Z0-9_]*/,

    // --- Identifiers ---

    identifier: (_) => /[a-zA-Z_][a-zA-Z0-9_]*/,

    // --- Comments ---

    comment: (_) => token(seq("#", /.*/)),
  },
});
