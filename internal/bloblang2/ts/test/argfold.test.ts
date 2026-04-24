// Verifies the parse-time argument-folding mechanism works end-to-end
// in the TS implementation. Mirrors the Go unit tests in
// ../../go/pratt/eval/argfold_test.go.
//
// Each test compiles a small mapping, then either inspects the
// resolved AST to confirm the folder fired, checks the resolver
// produced an error for an invalid literal, or runs the mapping and
// asserts the expected output.

import { describe, it, expect } from "vitest";
import { parse } from "../src/parser.js";
import { optimize } from "../src/optimizer.js";
import { resolve } from "../src/resolver.js";
import { Interpreter } from "../src/interpreter.js";
import { registerStdlib, stdlibNames } from "../src/stdlib/index.js";
import { mkString, toJSON } from "../src/value.js";
import type { PathExpr, Assignment } from "../src/ast.js";

function compile(src: string) {
  const { program, errors } = parse(src, "", null);
  if (errors.length > 0) throw new Error("parse: " + JSON.stringify(errors));
  optimize(program);
  const { methods, functions } = stdlibNames();
  const rerrs = resolve(program, methods, functions);
  return { program, rerrs };
}

function fresh(): Interpreter {
  const i = new Interpreter({ stmts: [], maps: [], imports: [], namespaces: new Map(), maxSlots: 0, readsOutput: false });
  return i;
}

describe("ArgFolder", () => {
  it("folds a literal regex pattern into a RegExp on the AST", () => {
    const { program, rerrs } = compile(`output = input.re_match("[0-9]+")`);
    expect(rerrs).toEqual([]);

    // Walk: Assignment.value is a PathExpr whose last segment is
    // .re_match(...). Expect seg.args[0].folded to be a RegExp.
    let found = false;
    const walk = (node: unknown) => {
      if (found || node === null || typeof node !== "object") return;
      const n = node as { kind?: string };
      if (n.kind === "path") {
        const p = node as PathExpr;
        for (const seg of p.segments) {
          if (seg.segKind === "method" && seg.name === "re_match") {
            if (seg.args.length > 0 && seg.args[0]!.folded instanceof RegExp) {
              found = true;
              return;
            }
          }
        }
      }
      if (n.kind === "assignment") {
        walk((node as Assignment).value);
      }
    };
    for (const s of program.stmts) walk(s);
    expect(found).toBe(true);

    // Runtime correctness.
    const interp = fresh();
    Object.assign(interp, new Interpreter(program));
    registerStdlib(interp);
    const result = interp.run(mkString("abc123"), new Map());
    expect(result.error).toBeFalsy();
    expect(toJSON(result.output)).toBe(true);
  });

  it("rejects an invalid regex literal at parse time", () => {
    const { rerrs } = compile(`output = input.re_match("[unclosed")`);
    expect(rerrs.length).toBeGreaterThan(0);
    const allMsgs = rerrs.map((e) => e.msg).join(" ");
    expect(allMsgs).toMatch(/re_match|invalid regex|Invalid regular expression/);
  });

  it("leaves dynamic patterns unfolded (compiled per call)", () => {
    const src = `$pat = "[0-9]+"\noutput = input.re_match($pat)`;
    const { program, rerrs } = compile(src);
    expect(rerrs).toEqual([]);

    // The re_match arg's folded should remain undefined because the
    // pattern came from a variable reference, not a literal.
    let sawArg = false;
    let argWasFolded = false;
    const walk = (node: unknown) => {
      if (node === null || typeof node !== "object") return;
      const n = node as { kind?: string };
      if (n.kind === "path") {
        const p = node as PathExpr;
        for (const seg of p.segments) {
          if (seg.segKind === "method" && seg.name === "re_match" && seg.args.length > 0) {
            sawArg = true;
            if (seg.args[0]!.folded !== undefined) argWasFolded = true;
          }
        }
      }
      if (n.kind === "assignment") walk((node as Assignment).value);
    };
    for (const s of program.stmts) walk(s);
    expect(sawArg).toBe(true);
    expect(argWasFolded).toBe(false);

    const interp = new Interpreter(program);
    registerStdlib(interp);
    const result = interp.run(mkString("abc123"), new Map());
    expect(result.error).toBeFalsy();
    expect(toJSON(result.output)).toBe(true);
  });
});
