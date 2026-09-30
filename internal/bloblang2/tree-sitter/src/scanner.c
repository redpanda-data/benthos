// External scanner for Bloblang V2 tree-sitter grammar.
//
// Handles newline significance: Bloblang uses newlines as statement separators,
// but suppresses them in certain contexts:
//
//   1. Inside () and [] — handled by the grammar (these rules don't include
//      _newline, so valid_symbols[NEWLINE] is false → scanner emits NL_SKIP).
//   2. After operators that can't end an expression (+, -, =, =>, ->, etc.) —
//      handled by the grammar (parser expects an expression, not _newline).
//   3. When the next line starts with a postfix token (., ?., [, ?[, else) —
//      handled HERE in the scanner via lookahead.
//   4. Consecutive newlines collapsed — handled HERE (emit one, skip the rest).
//
// Two external tokens:
//   NEWLINE  — significant newline (statement separator)
//   NL_SKIP  — newline consumed as whitespace (in extras)

#include "tree_sitter/parser.h"

#include <stdbool.h>

enum {
  NEWLINE = 0,
  NL_SKIP = 1,
};

// No persistent state needed — all decisions are based on valid_symbols
// and character lookahead.
void *tree_sitter_bloblang2_external_scanner_create(void) {
  return NULL;
}

void tree_sitter_bloblang2_external_scanner_destroy(void *payload) {
  (void)payload;
}

unsigned tree_sitter_bloblang2_external_scanner_serialize(void *payload,
                                                          char *buffer) {
  (void)payload;
  (void)buffer;
  return 0;
}

void tree_sitter_bloblang2_external_scanner_deserialize(void *payload,
                                                         const char *buffer,
                                                         unsigned length) {
  (void)payload;
  (void)buffer;
  (void)length;
}

// Peek ahead past whitespace and newlines to check for postfix continuation.
// Returns true if the next substantive token is '.', '?.', '[', '?[', or 'else'.
static bool is_postfix_continuation(TSLexer *lexer) {
  // We've already consumed the first \n. Now peek ahead.
  for (;;) {
    int32_t c = lexer->lookahead;
    if (c == ' ' || c == '\t' || c == '\r' || c == '\n') {
      lexer->advance(lexer, false);
      continue;
    }
    // Skip comment lines — a line starting with # is not a postfix continuation,
    // but we need to look past it to check the next real line.
    if (c == '#') {
      while (lexer->lookahead != '\n' && lexer->lookahead != 0) {
        lexer->advance(lexer, false);
      }
      continue;
    }
    break;
  }

  int32_t c = lexer->lookahead;

  if (c == '.') return true;
  if (c == '[') return true;
  if (c == '?') return true; // ?. or ?[

  // Check for 'else' keyword.
  if (c == 'e') {
    lexer->advance(lexer, false);
    if (lexer->lookahead != 'l') return false;
    lexer->advance(lexer, false);
    if (lexer->lookahead != 's') return false;
    lexer->advance(lexer, false);
    if (lexer->lookahead != 'e') return false;
    lexer->advance(lexer, false);
    // Must be word boundary.
    int32_t after = lexer->lookahead;
    if ((after >= 'a' && after <= 'z') ||
        (after >= 'A' && after <= 'Z') ||
        (after >= '0' && after <= '9') ||
        after == '_') {
      return false;
    }
    return true;
  }

  return false;
}

bool tree_sitter_bloblang2_external_scanner_scan(void *payload,
                                                  TSLexer *lexer,
                                                  const bool *valid_symbols) {
  (void)payload;

  // Skip spaces and tabs (but not newlines — those are what we're looking for).
  while (lexer->lookahead == ' ' || lexer->lookahead == '\t' ||
         lexer->lookahead == '\r') {
    lexer->advance(lexer, true);
  }

  // Not a newline — nothing for us to do.
  if (lexer->lookahead != '\n') {
    return false;
  }

  // We found a newline. Consume it.
  lexer->advance(lexer, false);

  // If the parser wants a significant newline (statement separator)...
  if (valid_symbols[NEWLINE]) {
    // Mark the end of the token here — everything after this is lookahead.
    lexer->mark_end(lexer);

    // Check for postfix continuation on the next line.
    if (is_postfix_continuation(lexer)) {
      // Suppress: emit as whitespace skip instead.
      lexer->result_symbol = NL_SKIP;
      return true;
    }

    // Emit significant newline.
    lexer->result_symbol = NEWLINE;
    return true;
  }

  // Parser doesn't want a newline here — consume as whitespace.
  if (valid_symbols[NL_SKIP]) {
    lexer->result_symbol = NL_SKIP;
    return true;
  }

  return false;
}
