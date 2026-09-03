// Unicode SIMPLE case mapping (per-codepoint 1:1), as required by the spec
// for .uppercase()/.lowercase() (Section 13.5).
//
// JavaScript's built-in toUpperCase()/toLowerCase() perform FULL case
// mapping ("ß" → "SS", "İ" → "i̇", final-sigma context rules), which
// can change a string's codepoint length and diverges from the Go engine.
// This module ports Go's unicode.to() lookup over tables generated from the
// Go toolchain's own unicode.CaseRanges (see scripts/gen-case-tables), so
// both engines agree by construction.

import { CASE_RANGES, UPPER_LOWER } from "./case_tables.js";

// Kind indices match Go's unicode.UpperCase / unicode.LowerCase.
const KIND_UPPER = 0;
const KIND_LOWER = 1;

// caseTo returns the simple case mapping of a single codepoint, mirroring
// Go's unicode.to(): binary search over CaseRanges, apply the delta, with
// the UPPER_LOWER sentinel marking alternating Upper,Lower sequences.
function caseTo(kind: number, cp: number): number {
  let lo = 0;
  let hi = CASE_RANGES.length / 4;
  while (lo < hi) {
    const m = (lo + hi) >> 1;
    const rLo = CASE_RANGES[m * 4]!;
    const rHi = CASE_RANGES[m * 4 + 1]!;
    if (rLo <= cp && cp <= rHi) {
      const delta = CASE_RANGES[m * 4 + 2 + kind]!;
      if (delta === UPPER_LOWER) {
        // Alternating Upper,Lower pairs: even offset is the upper form,
        // odd offset the lower. Select by kind.
        return rLo + (((cp - rLo) & ~1) | kind);
      }
      return cp + delta;
    }
    if (cp < rLo) {
      hi = m;
    } else {
      lo = m + 1;
    }
  }
  return cp;
}

function mapCodepoints(s: string, kind: number): string {
  let out = "";
  for (const ch of s) {
    out += String.fromCodePoint(caseTo(kind, ch.codePointAt(0)!));
  }
  return out;
}

// simpleUpper converts a string using Unicode simple uppercase mapping.
// Codepoint length is always preserved: "ß" stays "ß".
export function simpleUpper(s: string): string {
  return mapCodepoints(s, KIND_UPPER);
}

// simpleLower converts a string using Unicode simple lowercase mapping.
// Codepoint length is always preserved and no context rules apply:
// "Σ" becomes "σ" even in word-final position.
export function simpleLower(s: string): string {
  return mapCodepoints(s, KIND_LOWER);
}
