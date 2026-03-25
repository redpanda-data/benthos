// Scope for the Bloblang V2 interpreter.
//
// Two modes:
//   - "statement": assigning to an existing outer variable modifies it;
//     new variables are block-scoped.
//   - "expression": assigning always shadows (local); used for lambdas and maps.

import type { Value } from "./value.js";

export type ScopeMode = "statement" | "expression";

export class Scope {
  parent: Scope | null;
  mode: ScopeMode;
  vars: Map<string, Value>;

  constructor(parent: Scope | null, mode: ScopeMode) {
    this.parent = parent;
    this.mode = mode;
    this.vars = new Map();
  }

  /** Look up a variable by walking the scope chain. */
  get(name: string): Value | undefined {
    for (let cur: Scope | null = this; cur !== null; cur = cur.parent) {
      const v = cur.vars.get(name);
      if (v !== undefined) return v;
    }
    return undefined;
  }

  /**
   * Assign a variable, respecting the scope mode:
   *   - Expression mode: always writes locally (shadow).
   *   - Statement mode: if variable exists in an ancestor, update the ancestor.
   *     Otherwise, create locally.
   */
  set(name: string, value: Value): void {
    if (this.mode === "statement") {
      for (let cur = this.parent; cur !== null; cur = cur.parent) {
        if (cur.vars.has(name)) {
          cur.vars.set(name, value);
          return;
        }
      }
    }
    this.vars.set(name, value);
  }
}
