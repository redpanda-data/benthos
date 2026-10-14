package syntax

import "testing"

// FuzzParse exercises the parser with arbitrary bytes and asserts:
//
//  1. No panic, regardless of input.
//  2. Determinism: two parses of the same input produce the same error count.
//  3. Resolve never panics on a parsed program (errors are fine).
//  4. Print round-trip stability: for any cleanly-parsed input, the second
//     application of Parse → Print produces the same string as the first.
//  5. Optimize idempotence: running Optimize twice yields the same printed
//     output as running it once.
func FuzzParse(f *testing.F) {
	for _, s := range loadFuzzCorpus(f) {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		if len(src) > fuzzMaxInputSize {
			return
		}

		prog1, errs1 := Parse(src, "", nil)
		_, errs2 := Parse(src, "", nil)
		if len(errs1) != len(errs2) {
			t.Fatalf("non-deterministic parse: %d vs %d errors", len(errs1), len(errs2))
		}

		// Resolve must not panic; with empty opts every method/function
		// is unknown but errors-vs-no-errors is irrelevant here.
		_ = Resolve(prog1, ResolveOptions{})

		// Round-trip and idempotence properties only apply to clean parses.
		if len(errs1) > 0 {
			return
		}

		printed1 := Print(prog1)
		prog3, errs3 := Parse(printed1, "", nil)
		if len(errs3) > 0 {
			t.Fatalf("round-trip failed: re-parse of printed output errored\nprinted:\n%s\nerrors:\n%s", printed1, FormatErrors(errs3))
		}
		printed2 := Print(prog3)
		if printed1 != printed2 {
			t.Fatalf("Print not stable across round-trip:\nfirst:\n%s\nsecond:\n%s", printed1, printed2)
		}

		progA, _ := Parse(src, "", nil)
		Optimize(progA)
		printA := Print(progA)

		progB, _ := Parse(src, "", nil)
		Optimize(progB)
		Optimize(progB)
		printB := Print(progB)

		if printA != printB {
			t.Fatalf("Optimize not idempotent:\nonce:\n%s\ntwice:\n%s", printA, printB)
		}
	})
}
