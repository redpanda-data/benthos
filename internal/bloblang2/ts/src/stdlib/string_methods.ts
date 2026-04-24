// String methods: uppercase, lowercase, trim, trim_prefix, trim_suffix,
// has_prefix, has_suffix, split, replace_all, contains (string overload),
// repeat, re_match, re_find_all, re_replace_all, parse_int.

import type { Interpreter, MethodSpec } from "../interpreter.js";
import type { ArgFolder } from "../resolver.js";
import type { CallArg } from "../ast.js";
import { TokenType } from "../token.js";

/**
 * Convert Go regex replacement syntax to JS replacement syntax.
 * Go uses $0 for whole match, ${name} for named groups.
 * JS uses $& for whole match, $<name> for named groups.
 * We also need to escape $$ (literal $) properly.
 */
function goReplacementToJS(s: string): string {
  let result = "";
  for (let i = 0; i < s.length; i++) {
    if (s[i] === "$") {
      if (i + 1 < s.length && s[i + 1] === "0") {
        result += "$&";
        i++; // skip the '0'
      } else if (i + 1 < s.length && s[i + 1] === "{") {
        // Find closing brace.
        const close = s.indexOf("}", i + 2);
        if (close !== -1) {
          const name = s.substring(i + 2, close);
          result += "$<" + name + ">";
          i = close; // skip to '}'
        } else {
          result += s[i];
        }
      } else {
        result += s[i];
      }
    } else {
      result += s[i];
    }
  }
  return result;
}
import {
  type Value,
  mkString,
  mkBool,
  mkInt64,
  mkArray,
  mkError,
  isString,
  isInt64,
  isInt32,
  isUint32,
  isUint64,
  isFloat32,
  isFloat64,
  isFolded,
  typeName,
} from "../value.js";

function toInt64(v: Value): bigint | null {
  if (isInt64(v)) return v.value;
  if (isInt32(v)) return BigInt(v.value);
  if (isUint32(v)) return BigInt(v.value);
  if (isUint64(v)) return v.value;
  if (isFloat64(v)) return isFinite(v.value) ? BigInt(Math.trunc(v.value)) : null;
  if (isFloat32(v)) return isFinite(v.value) ? BigInt(Math.trunc(v.value)) : null;
  return null;
}

function requireString(
  methodName: string,
  receiver: Value,
): string | Value {
  if (!isString(receiver)) {
    return mkError(`${methodName}() requires string, got ${typeName(receiver)}`);
  }
  return receiver.value;
}

function requireStringArg(
  methodName: string,
  args: Value[],
  index: number,
): string | Value {
  const arg = args[index];
  if (arg === undefined || !isString(arg)) {
    return mkError(`${methodName}() argument must be string`);
  }
  return arg.value;
}

/**
 * foldRegexPattern is the ArgFolder shared by re_match, re_find_all,
 * and re_replace_all. If arg 0 is a string literal, it's compiled into
 * a RegExp at parse time (using `flags` — "" for re_match, "g" for the
 * two find/replace variants). Dynamic patterns (e.g. `.re_match($pat)`)
 * are left untouched and compile on every call, matching the previous
 * behaviour. Also applies the Go-to-JS syntax mapping for named
 * capture groups: `(?P<name>...)` -> `(?<name>...)`.
 */
function foldRegexPattern(flags: string): ArgFolder {
  return (args: CallArg[]): Array<unknown | null> => {
    const out: Array<unknown | null> = new Array(args.length).fill(null);
    if (args.length === 0) return out;
    const lit = args[0]!.value;
    if (lit.kind !== "literal") return out;
    if (lit.tokenType !== TokenType.STRING && lit.tokenType !== TokenType.RAW_STRING) {
      return out;
    }
    const jsPattern = String(lit.value).replace(/\(\?P</g, "(?<");
    try {
      out[0] = new RegExp(jsPattern, flags);
    } catch (e) {
      throw new Error(`invalid regex pattern ${JSON.stringify(lit.value)}: ${(e as Error).message}`);
    }
    return out;
  };
}

/**
 * resolveRegex extracts a RegExp from a pattern argument that may
 * already be precompiled (via foldRegexPattern) or still be a raw
 * string. Shared by the three re_* methods. Returns a RegExp on
 * success, or a Value (error) on failure.
 */
function resolveRegex(methodName: string, arg: Value, flags: string): RegExp | Value {
  if (isFolded(arg)) {
    if (arg.value instanceof RegExp) return arg.value;
    return mkError(`${methodName}() received folded value of unexpected type`);
  }
  if (!isString(arg)) {
    return mkError(`${methodName}() argument must be string`);
  }
  try {
    const jsPattern = arg.value.replace(/\(\?P</g, "(?<");
    return new RegExp(jsPattern, flags);
  } catch (e) {
    return mkError(`${methodName}() invalid pattern: ${(e as Error).message}`);
  }
}

export function registerStringMethods(interp: Interpreter): void {
  const m = (
    fn: (interp: Interpreter, receiver: Value, args: Value[]) => Value,
  ): MethodSpec => ({
    fn,
    lambdaFn: null,
    intrinsic: false,
    params: null,
    acceptsNull: false,
  });

  interp.registerMethod(
    "uppercase",
    m((_i, recv) => {
      const s = requireString("uppercase", recv);
      if (typeof s !== "string") return s;
      return mkString(s.toUpperCase());
    }),
  );

  interp.registerMethod(
    "lowercase",
    m((_i, recv) => {
      const s = requireString("lowercase", recv);
      if (typeof s !== "string") return s;
      return mkString(s.toLowerCase());
    }),
  );

  interp.registerMethod(
    "trim",
    m((_i, recv) => {
      const s = requireString("trim", recv);
      if (typeof s !== "string") return s;
      return mkString(s.trim());
    }),
  );

  interp.registerMethod(
    "trim_prefix",
    m((_i, recv, args) => {
      const s = requireString("trim_prefix", recv);
      if (typeof s !== "string") return s;
      if (args.length !== 1) return mkError("trim_prefix() requires one argument");
      const prefix = requireStringArg("trim_prefix", args, 0);
      if (typeof prefix !== "string") return prefix;
      return mkString(s.startsWith(prefix) ? s.slice(prefix.length) : s);
    }),
  );

  interp.registerMethod(
    "trim_suffix",
    m((_i, recv, args) => {
      const s = requireString("trim_suffix", recv);
      if (typeof s !== "string") return s;
      if (args.length !== 1) return mkError("trim_suffix() requires one argument");
      const suffix = requireStringArg("trim_suffix", args, 0);
      if (typeof suffix !== "string") return suffix;
      return mkString(
        s.endsWith(suffix) ? s.slice(0, s.length - suffix.length) : s,
      );
    }),
  );

  interp.registerMethod(
    "has_prefix",
    m((_i, recv, args) => {
      const s = requireString("has_prefix", recv);
      if (typeof s !== "string") return s;
      if (args.length !== 1) return mkError("has_prefix() requires one argument");
      const prefix = requireStringArg("has_prefix", args, 0);
      if (typeof prefix !== "string") return prefix;
      return mkBool(s.startsWith(prefix));
    }),
  );

  interp.registerMethod(
    "has_suffix",
    m((_i, recv, args) => {
      const s = requireString("has_suffix", recv);
      if (typeof s !== "string") return s;
      if (args.length !== 1) return mkError("has_suffix() requires one argument");
      const suffix = requireStringArg("has_suffix", args, 0);
      if (typeof suffix !== "string") return suffix;
      return mkBool(s.endsWith(suffix));
    }),
  );

  interp.registerMethod(
    "split",
    m((_i, recv, args) => {
      const s = requireString("split", recv);
      if (typeof s !== "string") return s;
      if (args.length !== 1) return mkError("split() requires one argument");
      const delim = requireStringArg("split", args, 0);
      if (typeof delim !== "string") return delim;

      if (delim === "") {
        if (s === "") return mkArray([]);
        // Split by codepoint.
        const codepoints = [...s];
        return mkArray(codepoints.map(mkString));
      }
      return mkArray(s.split(delim).map(mkString));
    }),
  );

  interp.registerMethod(
    "replace_all",
    m((_i, recv, args) => {
      const s = requireString("replace_all", recv);
      if (typeof s !== "string") return s;
      if (args.length !== 2) {
        return mkError("replace_all() requires old and new arguments");
      }
      const old = requireStringArg("replace_all", args, 0);
      if (typeof old !== "string") return old;
      const new_ = requireStringArg("replace_all", args, 1);
      if (typeof new_ !== "string") return new_;
      return mkString(s.replaceAll(old, new_));
    }),
  );

  interp.registerMethod(
    "repeat",
    m((_i, recv, args) => {
      const s = requireString("repeat", recv);
      if (typeof s !== "string") return s;
      if (args.length !== 1) return mkError("repeat() requires one argument");
      const count = toInt64(args[0]!);
      if (count === null) return mkError("repeat() argument must be integer");
      if (count < 0n) return mkError("repeat() count must be non-negative");
      if (count > 1_000_000n) return mkError("repeat() count too large");
      return mkString(s.repeat(Number(count)));
    }),
  );

  interp.registerMethod(
    "re_match",
    {
      fn: (_i, recv, args) => {
        const s = requireString("re_match", recv);
        if (typeof s !== "string") return s;
        if (args.length !== 1) return mkError("re_match() requires one argument");
        const re = resolveRegex("re_match", args[0]!, "");
        if (!(re instanceof RegExp)) return re;
        return mkBool(re.test(s));
      },
      lambdaFn: null,
      intrinsic: false,
      params: null,
      acceptsNull: false,
      argFolder: foldRegexPattern(""),
    },
  );

  interp.registerMethod(
    "re_find_all",
    {
      fn: (_i, recv, args) => {
        const s = requireString("re_find_all", recv);
        if (typeof s !== "string") return s;
        if (args.length !== 1) {
          return mkError("re_find_all() requires one argument");
        }
        const re = resolveRegex("re_find_all", args[0]!, "g");
        if (!(re instanceof RegExp)) return re;
        const matches = s.match(re);
        if (matches === null) return mkArray([]);
        return mkArray(matches.map(mkString));
      },
      lambdaFn: null,
      intrinsic: false,
      params: null,
      acceptsNull: false,
      argFolder: foldRegexPattern("g"),
    },
  );

  interp.registerMethod(
    "re_replace_all",
    {
      fn: (_i, recv, args) => {
        const s = requireString("re_replace_all", recv);
        if (typeof s !== "string") return s;
        if (args.length !== 2) {
          return mkError(
            "re_replace_all() requires pattern and replacement arguments",
          );
        }
        const re = resolveRegex("re_replace_all", args[0]!, "g");
        if (!(re instanceof RegExp)) return re;
        const replacement = requireStringArg("re_replace_all", args, 1);
        if (typeof replacement !== "string") return replacement;
        // Convert Go replacement syntax to JS:
        // $0 → $& (whole match), ${name} → $<name> (named group)
        const jsReplacement = goReplacementToJS(replacement);
        return mkString(s.replace(re, jsReplacement));
      },
      lambdaFn: null,
      intrinsic: false,
      params: null,
      acceptsNull: false,
      argFolder: foldRegexPattern("g"),
    },
  );

  interp.registerMethod(
    "parse_int",
    m((_i, recv) => {
      const s = requireString("parse_int", recv);
      if (typeof s !== "string") return s;
      try {
        const n = BigInt(s.trim());
        return mkInt64(n);
      } catch {
        return mkError("parse_int() cannot parse: " + s);
      }
    }),
  );
}
