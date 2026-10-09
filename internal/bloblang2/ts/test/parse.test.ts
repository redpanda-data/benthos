import { describe, it, expect } from "vitest";
import { readFileSync, readdirSync, statSync } from "fs";
import { join } from "path";
import { parse as parseYaml } from "yaml";
import { parse } from "../src/parser.js";

const SPEC_DIR = join(__dirname, "..", "..", "spec", "tests");

interface SpecFile {
  description?: string;
  files?: Record<string, string>;
  tests: SpecTest[];
}

interface SpecTest {
  name: string;
  mapping?: string;
  compile_error?: string;
  files?: Record<string, string>;
}

function collectYamlFiles(dir: string): string[] {
  const result: string[] = [];
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) {
      result.push(...collectYamlFiles(full));
    } else if (entry.endsWith(".yaml")) {
      result.push(full);
    }
  }
  return result;
}

const files = collectYamlFiles(SPEC_DIR);

describe("spec compatibility — parse", () => {
  for (const file of files) {
    const relPath = file.slice(SPEC_DIR.length + 1);
    const data = readFileSync(file, "utf-8");
    const spec = parseYaml(data) as SpecFile;

    for (const tc of spec.tests) {
      if (!tc.mapping || tc.compile_error) continue;

      const testName = `${relPath}/${tc.name}`;

      it(`parses: ${testName}`, () => {
        const filesMap = new Map<string, string>();
        if (spec.files) {
          for (const [name, content] of Object.entries(spec.files)) {
            filesMap.set(name, content);
          }
        }
        if (tc.files) {
          for (const [name, content] of Object.entries(tc.files)) {
            filesMap.set(name, content);
          }
        }
        const { errors } = parse(tc.mapping!, "", filesMap);
        if (errors.length > 0) {
          const msgs = errors.map(
            (e) => `  ${e.pos.line}:${e.pos.column}: ${e.msg}`,
          );
          expect.fail(
            `Parse errors in "${testName}":\n${msgs.join("\n")}\n\nMapping:\n${tc.mapping}`,
          );
        }
      });
    }
  }
});
