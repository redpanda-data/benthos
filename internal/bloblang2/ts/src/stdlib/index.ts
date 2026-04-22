// Stdlib entry point: registers all functions, methods, and lambda methods.

import type { Interpreter, MethodSpec, FunctionSpec } from "../interpreter.js";
import type { FunctionInfo, MethodInfo } from "../resolver.js";

import { registerFunctions } from "./functions.js";
import { registerTypeConversion } from "./type_conversion.js";
import { registerStringMethods } from "./string_methods.js";
import { registerArrayMethods } from "./array_methods.js";
import { registerObjectMethods } from "./object_methods.js";
import { registerNumericMethods } from "./numeric_methods.js";
import { registerEncoding } from "./encoding.js";
import { registerTimestamp } from "./timestamp.js";
import { registerLambdaMethods } from "./lambda_methods.js";

/**
 * Register all standard library functions and methods on the interpreter.
 */
export function registerStdlib(interp: Interpreter): void {
  // Functions.
  registerFunctions(interp);

  // Regular methods.
  registerTypeConversion(interp);
  registerStringMethods(interp);
  registerArrayMethods(interp);
  registerObjectMethods(interp);
  registerNumericMethods(interp);
  registerEncoding(interp);
  registerTimestamp(interp);

  // Lambda methods (higher-order).
  registerLambdaMethods(interp);
}

/**
 * Return the method and function registries needed by the resolver.
 * This creates a lightweight stub with just the registration surface,
 * avoiding a dependency on the full Interpreter constructor.
 *
 * Method infos carry arity (required / total). `required === 0` with
 * `total === -1` is used for methods that declare no params array
 * (e.g. intrinsic methods like `.catch` / `.or` whose arity is
 * handled specially at call sites).
 */
export function stdlibNames(): {
  methods: Map<string, MethodInfo>;
  functions: Map<string, FunctionInfo>;
} {
  // Minimal stub that satisfies registerStdlib's registration calls.
  const methods = new Map<string, MethodSpec>();
  const functions = new Map<string, FunctionSpec>();
  const stub = {
    methods,
    functions,
    maps: new Map(),
    namespaces: new Map(),
    registerMethod(name: string, spec: MethodSpec) {
      methods.set(name, spec);
    },
    registerFunction(name: string, spec: FunctionSpec) {
      functions.set(name, spec);
    },
  } as unknown as Interpreter;

  registerStdlib(stub);

  const methodInfos = new Map<string, MethodInfo>();
  for (const [name, spec] of methods) {
    if (!spec.params) {
      methodInfos.set(name, { required: 0, total: -1 });
      continue;
    }
    let required = 0;
    let total = 0;
    for (const p of spec.params) {
      total++;
      if (!p.hasDefault) required++;
    }
    methodInfos.set(name, { required, total });
  }

  const functionInfos = new Map<string, FunctionInfo>();
  for (const [name, spec] of functions) {
    let required = 0;
    let total = 0;
    for (const p of spec.params) {
      total++;
      if (!p.hasDefault) required++;
    }
    functionInfos.set(name, { required, total });
  }

  return { methods: methodInfos, functions: functionInfos };
}
