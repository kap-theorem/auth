package authz

import (
	"strings"
	"testing"
	"time"
)

func TestEvalCondition(t *testing.T) {
	fixedNow := func() time.Time {
		return time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	}

	cases := []struct {
		name string
		expr string
		ctx  map[string]string
		want bool
	}{
		// equality / inequality
		{"string eq true", `env == "staging"`, map[string]string{"env": "staging"}, true},
		{"string eq false", `env == "staging"`, map[string]string{"env": "prod"}, false},
		{"string neq", `env != "prod"`, map[string]string{"env": "staging"}, true},
		{"literal vs literal", `"a" == "a"`, nil, true},

		// numeric comparison (context values are strings but compare numerically)
		{"numeric lt", `attempts < 3`, map[string]string{"attempts": "2"}, true},
		{"numeric lt false", `attempts < 3`, map[string]string{"attempts": "3"}, false},
		{"numeric le", `attempts <= 3`, map[string]string{"attempts": "3"}, true},
		{"numeric gt", `score > 9.5`, map[string]string{"score": "9.75"}, true},
		{"numeric ge", `score >= 10`, map[string]string{"score": "10"}, true},
		{"numeric eq across formats", `n == 5.0`, map[string]string{"n": "5"}, true},
		{"negative number", `delta > -1`, map[string]string{"delta": "0"}, true},

		// non-numeric falls back to lexicographic strings
		{"string lt", `name < "m"`, map[string]string{"name": "alice"}, true},

		// boolean operators and parentheses
		{"and both true", `env == "staging" && attempts < 3`, map[string]string{"env": "staging", "attempts": "1"}, true},
		{"and one false", `env == "staging" && attempts < 3`, map[string]string{"env": "prod", "attempts": "1"}, false},
		{"or", `env == "staging" || env == "dev"`, map[string]string{"env": "dev"}, true},
		{"or both false", `env == "staging" || env == "dev"`, map[string]string{"env": "prod"}, false},
		{"parentheses", `(tier == "gold" || tier == "silver") && env == "prod"`, map[string]string{"tier": "silver", "env": "prod"}, true},
		{"precedence: && binds tighter than ||", `a == "1" || a == "2" && a == "3"`, map[string]string{"a": "1"}, true},

		// now()
		{"now before literal", `now() < "2027-01-01T00:00:00Z"`, nil, true},
		{"now after literal", `now() < "2020-01-01T00:00:00Z"`, nil, false},
		{"now in window", `now() >= "2026-07-18T00:00:00Z" && now() < "2026-07-19T00:00:00Z"`, nil, true},

		// fail closed
		{"missing context key", `env == "staging"`, map[string]string{}, false},
		{"missing key on one || branch fails whole expr", `a == "1" || missing == "x"`, map[string]string{"a": "1"}, false},
		{"parse error: garbage", `env == = broken`, map[string]string{"env": "x"}, false},
		{"parse error: bare identifier", `env`, map[string]string{"env": "x"}, false},
		{"parse error: unterminated string", `env == "staging`, map[string]string{"env": "staging"}, false},
		{"parse error: unbalanced paren", `(env == "x"`, map[string]string{"env": "x"}, false},
		{"parse error: trailing tokens", `env == "x" env`, map[string]string{"env": "x"}, false},
		{"parse error: single &", `env == "x" & env == "x"`, map[string]string{"env": "x"}, false},
		{"empty expression", ``, map[string]string{"env": "x"}, false},

		// escapes
		{"escaped quote in string", `msg == "say \"hi\""`, map[string]string{"msg": `say "hi"`}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := evalConditionAt(tc.expr, tc.ctx, fixedNow); got != tc.want {
				t.Fatalf("evalConditionAt(%q, %v) = %v, want %v", tc.expr, tc.ctx, got, tc.want)
			}
		})
	}
}

func TestParseModelValidation(t *testing.T) {
	cases := []struct {
		name    string
		json    string
		wantErr string // substring; "" = must parse
	}{
		{"valid model", fixtureModelJSON, ""},
		{"empty object", `{}`, ""},
		{"empty types", `{"types": {}}`, ""},
		{"not an object", `["types"]`, "JSON object"},
		{"malformed json", `{"types":`, "JSON object"},
		{"unknown top-level key", `{"types": {}, "extra": 1}`, "unknown"},
		{
			"unknown relation in implication list",
			`{"types": {"doc": {"relations": {"viewer": ["editor"]}}}}`,
			`unknown relation "editor"`,
		},
		{
			"self implication cycle",
			`{"types": {"doc": {"relations": {"viewer": ["viewer"]}}}}`,
			"cycle",
		},
		{
			"two-relation cycle",
			`{"types": {"doc": {"relations": {"a": ["b"], "b": ["a"]}}}}`,
			"cycle",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseModel(tc.json)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ParseModel failed: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ParseModel accepted an invalid model")
			}
			if !containsFold(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestModelExpand(t *testing.T) {
	model, err := ParseModel(fixtureModelJSON)
	if err != nil {
		t.Fatalf("ParseModel failed: %v", err)
	}
	got := model.Expand("problem", "viewer")
	for _, rel := range []string{"viewer", "editor", "author"} {
		if !got[rel] {
			t.Fatalf("Expand(problem, viewer) is missing %q: %v", rel, got)
		}
	}
	if len(got) != 3 {
		t.Fatalf("Expand(problem, viewer) = %v, want exactly viewer/editor/author", got)
	}
	if got := model.Expand("problem", "author"); len(got) != 1 || !got["author"] {
		t.Fatalf("Expand(problem, author) = %v, want just author", got)
	}
	if got := model.Expand("unknown_type", "viewer"); len(got) != 1 || !got["viewer"] {
		t.Fatalf("Expand on unknown type = %v, want just the relation itself", got)
	}
	var nilModel *Model
	if got := nilModel.Expand("problem", "viewer"); len(got) != 1 || !got["viewer"] {
		t.Fatalf("Expand on nil model = %v, want just the relation itself", got)
	}
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
