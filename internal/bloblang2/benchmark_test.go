package bloblang2

import (
	"encoding/json"
	"testing"

	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/eval"
	"github.com/redpanda-data/benthos/v4/internal/bloblang2/go/pratt/syntax"
	"github.com/redpanda-data/benthos/v4/public/bloblang"

	// Register v1 bloblang methods (timestamps, etc.) that live outside the
	// core query package.
	_ "github.com/redpanda-data/benthos/v4/internal/impl/pure"
)

// stripeInput is the Stripe invoice.paid webhook payload used by both benchmarks.
// All integers use int64 because the v2 type system requires explicit integer
// width (it does not recognise Go's plain int).
var stripeInput = map[string]any{
	"type":    "invoice.paid",
	"created": int64(1709251200),
	"data": map[string]any{
		"object": map[string]any{
			"id":             "in_1OqR3m",
			"number":         "INV-2024-0218",
			"customer":       "cus_PaB3xK",
			"customer_email": "ops@megacorp.io",
			"customer_name":  "MegaCorp Engineering",
			"metadata": map[string]any{
				"internal_account_id": "acct-00482",
				"provisioning":        `{"tier":"growth","seats":25,"features":["sso","audit_log"]}`,
				"salesforce_opp_id":   "006Dn000002XLPQ",
			},
			"status":       "paid",
			"subscription": "sub_1NrT7a",
			"currency":     "usd",
			"subtotal":     int64(14900),
			"tax":          int64(1200),
			"total":        int64(16100),
			"status_transitions": map[string]any{
				"paid_at":   int64(1709251200),
				"voided_at": nil,
			},
			"lines": map[string]any{
				"data": []any{
					map[string]any{
						"amount":      int64(9900),
						"description": "Growth Plan",
						"quantity":    int64(1),
						"product":     "prod_Growth",
					},
					map[string]any{
						"amount":      int64(5000),
						"description": "Extra seats",
						"quantity":    int64(5),
						"product":     "prod_Seats",
					},
				},
			},
		},
	},
}

// githubInput is the GitHub PR webhook payload used by both benchmarks.
var githubInput = map[string]any{
	"action": "opened",
	"number": int64(42),
	"pull_request": map[string]any{
		"title":         "feat: add retry logic to payment processor",
		"body":          "## Summary\nAdds exponential backoff.\n\nCloses #38\nRelated: #35, #40",
		"html_url":      "https://github.com/acme/payments/pull/42",
		"state":         "open",
		"draft":         false,
		"additions":     int64(347),
		"deletions":     int64(42),
		"changed_files": int64(8),
		"created_at":    "2024-01-15T14:30:00Z",
		"user":          map[string]any{"login": "alice-dev"},
		"head":          map[string]any{"ref": "feat/payment-retry"},
		"base":          map[string]any{"ref": "main"},
		"labels": []any{
			map[string]any{"name": "enhancement"},
			map[string]any{"name": "payments"},
			map[string]any{"name": "needs-review"},
		},
		"requested_reviewers": []any{
			map[string]any{"login": "bob-reviewer"},
			map[string]any{"login": "carol-lead"},
		},
		"requested_teams": []any{
			map[string]any{"name": "platform-team"},
		},
	},
}

// ---------------------------------------------------------------------------
// Bloblang V2 benchmarks
// ---------------------------------------------------------------------------

const v2StripeMapping = `
$inv = input.data.object

$provisioning = $inv.metadata.provisioning.parse_json()

$line_items = $inv.lines.data.map(item -> {
  "description": item.description,
  "amount_dollars": item.amount.float64() / 100.0,
  "quantity": item.quantity,
  "product_id": item.product,
})

output.invoice_id = $inv.id
output.invoice_number = $inv.number
output.event_type = input.type
output.customer = {
  "id": $inv.customer,
  "name": $inv.customer_name,
  "email": $inv.customer_email,
}
output.provisioning = $provisioning
output.currency = $inv.currency.uppercase()
output.subtotal_dollars = $inv.subtotal.float64() / 100.0
output.tax_dollars = $inv.tax.float64() / 100.0
output.total_dollars = $inv.total.float64() / 100.0
output.line_items = $line_items
output.status = $inv.status

output.paid_at = $inv.status_transitions.paid_at
  .ts_from_unix().string()

output.subscription_id = $inv.subscription

output.external_refs = $inv.metadata
  .without(["provisioning"])
  .map_keys(k -> k.replace_all("_", "-"))
`

const v2GithubMapping = `
$pr = input.pull_request
$url_parts = $pr.html_url.split("/")
$repo = $url_parts[3] + "/" + $url_parts[4]

$total_changes = $pr.additions + $pr.deletions
$size_category = match {
  $total_changes > 300 => "large",
  $total_changes > 100 => "medium",
  _ => "small",
}

$issue_refs = $pr.body.re_find_all("#\\d+")
  .map(ref -> ref.trim_prefix("#").int64())
  .sort()
  .unique()

$reviewers = $pr.requested_reviewers.map(r -> r.login)
  .concat($pr.requested_teams.map(t -> t.name))
  .sort()

output.event_type = "pr_" + input.action
output.repo = $repo
output.pr_number = input.number
output.title = $pr.title
output.author = $pr.user.login
output.url = $pr.html_url
output.branch = $pr.head.ref + " -> " + $pr.base.ref
output.labels = $pr.labels.map(l -> l.name).sort()
output.reviewers = $reviewers
output.size = {
  "additions": $pr.additions,
  "deletions": $pr.deletions,
  "files": $pr.changed_files,
  "category": $size_category,
}
output.referenced_issues = $issue_refs
output.is_feature = $pr.title.has_prefix("feat")
output.summary = "[" + $repo + "] " + $pr.user.login + " " + input.action + " #" + input.number.string() + ": " + $pr.title + " (" + $size_category + ", " + $pr.changed_files.string() + " files)"
`

// jsonNormalize round-trips a value through JSON so that both v1 and v2
// outputs use the same numeric types (float64 for all numbers) and can be
// compared with reflect.DeepEqual via their JSON representation.
func jsonNormalize(t testing.TB, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	// Re-indent for readable diffs on failure.
	var pretty json.RawMessage = b
	out, err := json.MarshalIndent(pretty, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent: %v", err)
	}
	return string(out)
}

func compileV2Impl(t testing.TB, mapping string) *compiledMapping {
	t.Helper()
	prog, errs := syntax.Parse(mapping, "", nil)
	if len(errs) > 0 {
		t.Fatalf("v2 parse errors: %s", syntax.FormatErrors(errs))
	}
	syntax.Optimize(prog)
	methods, functions := eval.StdlibNames()
	methodOpcodes, functionOpcodes := eval.StdlibOpcodes()
	if resolveErrs := syntax.Resolve(prog, syntax.ResolveOptions{
		Methods:         methods,
		Functions:       functions,
		MethodOpcodes:   methodOpcodes,
		FunctionOpcodes: functionOpcodes,
	}); len(resolveErrs) > 0 {
		t.Fatalf("v2 resolve errors: %s", syntax.FormatErrors(resolveErrs))
	}
	return &compiledMapping{
		prog:   prog,
		interp: eval.NewWithStdlib(prog),
	}
}

func compileV2T(t testing.TB, mapping string) *compiledMapping {
	return compileV2Impl(t, mapping)
}

func compileV1T(t testing.TB, mapping string) *bloblang.Executor {
	t.Helper()
	exec, err := bloblang.Parse(mapping)
	if err != nil {
		t.Fatalf("v1 parse error: %v", err)
	}
	return exec
}

func compileV2(b *testing.B, mapping string) *compiledMapping {
	return compileV2Impl(b, mapping)
}

func BenchmarkV2StripeInvoice(b *testing.B) {
	m := compileV2(b, v2StripeMapping)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		out, _, _, err := m.Exec(stripeInput, nil)
		if err != nil {
			b.Fatal(err)
		}
		if out == nil {
			b.Fatal("nil output")
		}
	}
}

func BenchmarkV2GithubWebhook(b *testing.B) {
	m := compileV2(b, v2GithubMapping)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		out, _, _, err := m.Exec(githubInput, nil)
		if err != nil {
			b.Fatal(err)
		}
		if out == nil {
			b.Fatal("nil output")
		}
	}
}

// ---------------------------------------------------------------------------
// Bloblang V1 benchmarks (same transformations)
// ---------------------------------------------------------------------------

const v1StripeMapping = `
let inv = this.data.object

let provisioning = $inv.metadata.provisioning.parse_json()

let line_items = $inv.lines.data.map_each(item -> {
  "description": item.description,
  "amount_dollars": item.amount.number() / 100,
  "quantity": item.quantity,
  "product_id": item.product,
})

root.invoice_id = $inv.id
root.invoice_number = $inv.number
root.event_type = this.type
root.customer = {
  "id": $inv.customer,
  "name": $inv.customer_name,
  "email": $inv.customer_email,
}
root.provisioning = $provisioning
root.currency = $inv.currency.uppercase()
root.subtotal_dollars = $inv.subtotal.number() / 100
root.tax_dollars = $inv.tax.number() / 100
root.total_dollars = $inv.total.number() / 100
root.line_items = $line_items
root.status = $inv.status

root.paid_at = $inv.status_transitions.paid_at.ts_format("2006-01-02T15:04:05Z", "UTC")

root.subscription_id = $inv.subscription

root.external_refs = $inv.metadata.without("provisioning").map_each_key(k -> k.replace_all("_", "-"))
`

const v1GithubMapping = `
let pr = this.pull_request
let repo = $pr.html_url.re_find_all("[^/]+").slice(2, 4).join("/")

let total_changes = $pr.additions + $pr.deletions
let size_category = if $total_changes > 300 { "large" } else if $total_changes > 100 { "medium" } else { "small" }

let issue_refs = $pr.body.re_find_all("#\\d+").map_each(ref -> ref.trim_prefix("#").number()).sort().unique()

let reviewers = $pr.requested_reviewers.map_each(r -> r.login).merge($pr.requested_teams.map_each(t -> t.name)).sort()

root.event_type = "pr_" + this.action
root.repo = $repo
root.pr_number = this.number
root.title = $pr.title
root.author = $pr.user.login
root.url = $pr.html_url
root.branch = $pr.head.ref + " -> " + $pr.base.ref
root.labels = $pr.labels.map_each(l -> l.name).sort()
root.reviewers = $reviewers
root.size = {
  "additions": $pr.additions,
  "deletions": $pr.deletions,
  "files": $pr.changed_files,
  "category": $size_category,
}
root.referenced_issues = $issue_refs
root.is_feature = $pr.title.has_prefix("feat")
root.summary = "[" + $repo + "] " + $pr.user.login + " " + this.action + " #" + this.number.string() + ": " + $pr.title + " (" + $size_category + ", " + $pr.changed_files.string() + " files)"
`

func compileV1(b *testing.B, mapping string) *bloblang.Executor {
	b.Helper()

	exec, err := bloblang.Parse(mapping)
	if err != nil {
		b.Fatalf("v1 parse error: %v", err)
	}
	return exec
}

func BenchmarkV1StripeInvoice(b *testing.B) {
	exec := compileV1(b, v1StripeMapping)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		out, err := exec.Query(stripeInput)
		if err != nil {
			b.Fatal(err)
		}
		if out == nil {
			b.Fatal("nil output")
		}
	}
}

func BenchmarkV1GithubWebhook(b *testing.B) {
	exec := compileV1(b, v1GithubMapping)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		out, err := exec.Query(githubInput)
		if err != nil {
			b.Fatal(err)
		}
		if out == nil {
			b.Fatal("nil output")
		}
	}
}

// ---------------------------------------------------------------------------
// Expected outputs (from the spec case studies, JSON-normalized)
// ---------------------------------------------------------------------------

const expectedStripeJSON = `{
  "currency": "USD",
  "customer": {
    "email": "ops@megacorp.io",
    "id": "cus_PaB3xK",
    "name": "MegaCorp Engineering"
  },
  "event_type": "invoice.paid",
  "external_refs": {
    "internal-account-id": "acct-00482",
    "salesforce-opp-id": "006Dn000002XLPQ"
  },
  "invoice_id": "in_1OqR3m",
  "invoice_number": "INV-2024-0218",
  "line_items": [
    {
      "amount_dollars": 99,
      "description": "Growth Plan",
      "product_id": "prod_Growth",
      "quantity": 1
    },
    {
      "amount_dollars": 50,
      "description": "Extra seats",
      "product_id": "prod_Seats",
      "quantity": 5
    }
  ],
  "paid_at": "2024-03-01T00:00:00Z",
  "provisioning": {
    "features": [
      "sso",
      "audit_log"
    ],
    "seats": 25,
    "tier": "growth"
  },
  "status": "paid",
  "subscription_id": "sub_1NrT7a",
  "subtotal_dollars": 149,
  "tax_dollars": 12,
  "total_dollars": 161
}`

const expectedGithubJSON = `{
  "author": "alice-dev",
  "branch": "feat/payment-retry -> main",
  "event_type": "pr_opened",
  "is_feature": true,
  "labels": [
    "enhancement",
    "needs-review",
    "payments"
  ],
  "pr_number": 42,
  "referenced_issues": [
    35,
    38,
    40
  ],
  "repo": "acme/payments",
  "reviewers": [
    "bob-reviewer",
    "carol-lead",
    "platform-team"
  ],
  "size": {
    "additions": 347,
    "category": "large",
    "deletions": 42,
    "files": 8
  },
  "summary": "[acme/payments] alice-dev opened #42: feat: add retry logic to payment processor (large, 8 files)",
  "title": "feat: add retry logic to payment processor",
  "url": "https://github.com/acme/payments/pull/42"
}`

// ---------------------------------------------------------------------------
// Output validation tests
// ---------------------------------------------------------------------------

func TestBenchmarkOutputs(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"Stripe", expectedStripeJSON},
		{"Github", expectedGithubJSON},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v2Mapping, v1Mapping string
			var input map[string]any
			switch tt.name {
			case "Stripe":
				v2Mapping, v1Mapping, input = v2StripeMapping, v1StripeMapping, stripeInput
			case "Github":
				v2Mapping, v1Mapping, input = v2GithubMapping, v1GithubMapping, githubInput
			}

			// Normalize expected output via JSON round-trip (sorts keys,
			// collapses integer types to float64).
			want := jsonNormalize(t, json.RawMessage(tt.expected))

			t.Run("V2", func(t *testing.T) {
				m := compileV2T(t, v2Mapping)
				out, _, _, err := m.Exec(input, nil)
				if err != nil {
					t.Fatalf("exec: %v", err)
				}
				got := jsonNormalize(t, out)
				if got != want {
					t.Fatalf("output mismatch\nwant:\n%s\ngot:\n%s", want, got)
				}
			})

			t.Run("V1", func(t *testing.T) {
				exec := compileV1T(t, v1Mapping)
				out, err := exec.Query(input)
				if err != nil {
					t.Fatalf("exec: %v", err)
				}
				got := jsonNormalize(t, out)
				if got != want {
					t.Fatalf("output mismatch\nwant:\n%s\ngot:\n%s", want, got)
				}
			})
		})
	}
}
