package authz

import (
	"authservice/pkg/models"
	"context"
	"fmt"
	"strings"
	"testing"
)

// memStore is an in-memory TupleStore for fixtures.
type memStore struct {
	tuples []models.RelationTuple
}

func (s *memStore) TuplesForObject(_ context.Context, clientID, objectType, objectID string) ([]models.RelationTuple, error) {
	var out []models.RelationTuple
	for _, t := range s.tuples {
		if t.ClientID == clientID && t.ObjectType == objectType && t.ObjectID == objectID {
			out = append(out, t)
		}
	}
	return out, nil
}

func (s *memStore) TuplesBySubject(_ context.Context, clientID, subjectType, subjectID, objectType string) ([]models.RelationTuple, error) {
	var out []models.RelationTuple
	for _, t := range s.tuples {
		if t.ClientID == clientID && t.SubjectType == subjectType && t.SubjectID == subjectID && t.ObjectType == objectType {
			out = append(out, t)
		}
	}
	return out, nil
}

// tup builds a fixture tuple. extras: [effect] then [condition_expr].
func tup(clientID, objectType, objectID, relation, subjectType, subjectID string, extras ...string) models.RelationTuple {
	t := models.RelationTuple{
		ClientID:    clientID,
		ObjectType:  objectType,
		ObjectID:    objectID,
		Relation:    relation,
		SubjectType: subjectType,
		SubjectID:   subjectID,
		Effect:      models.EffectAllow,
	}
	if len(extras) > 0 {
		t.Effect = extras[0]
	}
	if len(extras) > 1 {
		t.ConditionExpr = extras[1]
	}
	return t
}

// The fixture mirrors the console mock's dsapanicle seed data
// (console/src/api/mock.ts) plus deny/condition/nesting cases:
//
//	problem relations: author ⇒ editor ⇒ viewer
//	app relations:     admin ⇒ moderator ⇒ member
const fixtureModelJSON = `{
  "types": {
    "app":     {"relations": {"admin": [], "moderator": ["admin"], "member": ["moderator"]}},
    "problem": {"relations": {"author": [], "editor": ["author"], "viewer": ["editor"]}}
  }
}`

const clientA = "client_a"
const clientB = "client_b"

func fixtureTuples() []models.RelationTuple {
	return []models.RelationTuple{
		// direct + implications
		tup(clientA, "problem", "two-sum", "author", "user", "kush"),
		// userset: moderators can edit two-sum; mina is a moderator
		tup(clientA, "problem", "two-sum", "editor", "role", "moderator"),
		tup(clientA, "role", "moderator", "member", "user", "mina"),
		// nested userset: every senior is a moderator; omar is a senior
		tup(clientA, "role", "moderator", "member", "role", "senior"),
		tup(clientA, "role", "senior", "member", "user", "omar"),
		// deny-wins: banned holds editor but is explicitly denied viewer
		tup(clientA, "problem", "two-sum", "editor", "user", "banned"),
		tup(clientA, "problem", "two-sum", "viewer", "user", "banned", models.EffectDeny),
		// deny on author only (must not cascade down the implication chain)
		tup(clientA, "problem", "binary-search", "author", "user", "lena"),
		tup(clientA, "problem", "binary-search", "author", "user", "lena", models.EffectDeny),
		// conditions
		tup(clientA, "problem", "binary-search", "viewer", "user", "contractor", models.EffectAllow, `env == "staging"`),
		tup(clientA, "problem", "binary-search", "viewer", "user", "glitch", models.EffectAllow, `env == = broken`),
		tup(clientA, "problem", "binary-search", "viewer", "user", "temp"),
		tup(clientA, "problem", "binary-search", "viewer", "user", "temp", models.EffectDeny, `env == "prod"`),
		// cross-client isolation: same coordinates under another client
		tup(clientB, "problem", "two-sum", "viewer", "user", "intruder"),
	}
}

func newFixture(t *testing.T) (*Resolver, *Model) {
	t.Helper()
	model, err := ParseModel(fixtureModelJSON)
	if err != nil {
		t.Fatalf("fixture model failed to parse: %v", err)
	}
	return NewResolver(&memStore{tuples: fixtureTuples()}), model
}

// TestResolverCheck is the spec §10 resolver table: the most important test
// artifact in the project.
func TestResolverCheck(t *testing.T) {
	cases := []struct {
		name         string
		clientID     string
		subject      string // "type:id"
		relation     string
		object       string // "type:id"
		ctx          map[string]string
		wantAllowed  bool
		wantReasonIn string // substring the reason must contain (optional)
	}{
		{
			name:     "direct allow",
			clientID: clientA, subject: "user:kush", relation: "author", object: "problem:two-sum",
			wantAllowed: true, wantReasonIn: "matched tuple",
		},
		{
			name:     "implication one hop: author grants editor",
			clientID: clientA, subject: "user:kush", relation: "editor", object: "problem:two-sum",
			wantAllowed: true,
		},
		{
			name:     "implication two hops: author grants viewer",
			clientID: clientA, subject: "user:kush", relation: "viewer", object: "problem:two-sum",
			wantAllowed: true,
		},
		{
			name:     "relation not implied: editor tuple does not grant author",
			clientID: clientA, subject: "user:mina", relation: "author", object: "problem:two-sum",
			wantAllowed: false, wantReasonIn: "default deny",
		},
		{
			name:     "userset hop: moderator role grants editor",
			clientID: clientA, subject: "user:mina", relation: "editor", object: "problem:two-sum",
			wantAllowed: true, wantReasonIn: "via role:moderator",
		},
		{
			name:     "userset hop + implication: moderator role grants viewer",
			clientID: clientA, subject: "user:mina", relation: "viewer", object: "problem:two-sum",
			wantAllowed: true,
		},
		{
			name:     "nested userset: senior ⊂ moderator grants editor",
			clientID: clientA, subject: "user:omar", relation: "editor", object: "problem:two-sum",
			wantAllowed: true, wantReasonIn: "via role:moderator",
		},
		{
			name:     "deny wins over allow",
			clientID: clientA, subject: "user:banned", relation: "viewer", object: "problem:two-sum",
			wantAllowed: false, wantReasonIn: "explicit deny",
		},
		{
			name:     "deny does not cascade through implications (deny on author, check editor)",
			clientID: clientA, subject: "user:lena", relation: "editor", object: "problem:binary-search",
			wantAllowed: true,
		},
		{
			name:     "denied relation itself stays denied",
			clientID: clientA, subject: "user:lena", relation: "author", object: "problem:binary-search",
			wantAllowed: false, wantReasonIn: "explicit deny",
		},
		{
			name:     "condition true → conditioned allow allows",
			clientID: clientA, subject: "user:contractor", relation: "viewer", object: "problem:binary-search",
			ctx:         map[string]string{"env": "staging"},
			wantAllowed: true,
		},
		{
			name:     "condition false → conditioned allow ignored",
			clientID: clientA, subject: "user:contractor", relation: "viewer", object: "problem:binary-search",
			ctx:         map[string]string{"env": "prod"},
			wantAllowed: false, wantReasonIn: "default deny",
		},
		{
			name:     "missing context key → fail closed",
			clientID: clientA, subject: "user:contractor", relation: "viewer", object: "problem:binary-search",
			ctx:         map[string]string{},
			wantAllowed: false,
		},
		{
			name:     "condition parse error → fail closed",
			clientID: clientA, subject: "user:glitch", relation: "viewer", object: "problem:binary-search",
			ctx:         map[string]string{"env": "staging"},
			wantAllowed: false,
		},
		{
			name:     "conditioned deny true → denies despite unconditioned allow",
			clientID: clientA, subject: "user:temp", relation: "viewer", object: "problem:binary-search",
			ctx:         map[string]string{"env": "prod"},
			wantAllowed: false, wantReasonIn: "explicit deny",
		},
		{
			name:     "conditioned deny false → deny tuple ignored, allow stands",
			clientID: clientA, subject: "user:temp", relation: "viewer", object: "problem:binary-search",
			ctx:         map[string]string{"env": "staging"},
			wantAllowed: true,
		},
		{
			name:     "default deny: unknown subject",
			clientID: clientA, subject: "user:stranger", relation: "viewer", object: "problem:two-sum",
			wantAllowed: false, wantReasonIn: "default deny",
		},
		{
			name:     "default deny: unknown object",
			clientID: clientA, subject: "user:kush", relation: "viewer", object: "problem:no-such",
			wantAllowed: false, wantReasonIn: "default deny",
		},
		{
			name:     "cross-client isolation: client B tuple invisible in client A",
			clientID: clientA, subject: "user:intruder", relation: "viewer", object: "problem:two-sum",
			wantAllowed: false,
		},
		{
			name:     "cross-client isolation: client A tuple invisible in client B",
			clientID: clientB, subject: "user:kush", relation: "author", object: "problem:two-sum",
			wantAllowed: false,
		},
	}

	resolver, model := newFixture(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			subType, subID, _ := strings.Cut(tc.subject, ":")
			objType, objID, _ := strings.Cut(tc.object, ":")
			allowed, reason, err := resolver.Check(context.Background(), tc.clientID, model, subType, subID, tc.relation, objType, objID, tc.ctx)
			if err != nil {
				t.Fatalf("Check returned error: %v", err)
			}
			if allowed != tc.wantAllowed {
				t.Fatalf("Check = %v (reason: %s), want %v", allowed, reason, tc.wantAllowed)
			}
			if tc.wantReasonIn != "" && !strings.Contains(reason, tc.wantReasonIn) {
				t.Fatalf("reason %q does not contain %q", reason, tc.wantReasonIn)
			}
		})
	}
}

func TestResolverDepthLimit(t *testing.T) {
	// role_0 ← role_1 ← ... ← role_25 ← user:deep; doc grants via role_0.
	// Resolving user:deep needs 26 hops — beyond MaxDepth.
	var tuples []models.RelationTuple
	tuples = append(tuples, tup(clientA, "doc", "spec", "viewer", "role", "role_0"))
	const chain = MaxDepth + 5
	for i := 0; i < chain; i++ {
		tuples = append(tuples, tup(clientA, "role", fmt.Sprintf("role_%d", i), "member", "role", fmt.Sprintf("role_%d", i+1)))
	}
	tuples = append(tuples, tup(clientA, "role", fmt.Sprintf("role_%d", chain), "member", "user", "deep"))
	resolver := NewResolver(&memStore{tuples: tuples})

	allowed, reason, err := resolver.Check(context.Background(), clientA, nil, "user", "deep", "viewer", "doc", "spec", nil)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if allowed {
		t.Fatalf("expected deny past the depth limit")
	}
	if !strings.Contains(reason, "depth limit") {
		t.Fatalf("reason %q does not mention the depth limit", reason)
	}

	// A shallow membership chain still resolves.
	allowed, _, err = resolver.Check(context.Background(), clientA, nil, "role", "role_3", "viewer", "doc", "spec", nil)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if !allowed {
		t.Fatalf("expected allow within the depth limit")
	}
}

func TestResolverNoModel(t *testing.T) {
	// Without a model, only exact relations match (no implications).
	resolver := NewResolver(&memStore{tuples: []models.RelationTuple{
		tup(clientA, "problem", "two-sum", "author", "user", "kush"),
	}})
	allowed, _, err := resolver.Check(context.Background(), clientA, nil, "user", "kush", "author", "problem", "two-sum", nil)
	if err != nil || !allowed {
		t.Fatalf("expected exact-relation allow without a model (allowed=%v, err=%v)", allowed, err)
	}
	allowed, _, err = resolver.Check(context.Background(), clientA, nil, "user", "kush", "viewer", "problem", "two-sum", nil)
	if err != nil || allowed {
		t.Fatalf("expected deny for implied relation without a model (allowed=%v, err=%v)", allowed, err)
	}
}

func TestListObjects(t *testing.T) {
	resolver, model := newFixture(t)
	ctx := context.Background()

	cases := []struct {
		name     string
		subject  string
		relation string
		want     []string
	}{
		// kush authors two-sum only.
		{"direct + implication", "user:kush", "viewer", []string{"two-sum"}},
		// mina reaches two-sum via role:moderator.
		{"userset reachability", "user:mina", "editor", []string{"two-sum"}},
		// omar reaches two-sum via nested role senior → moderator.
		{"nested userset reachability", "user:omar", "viewer", []string{"two-sum"}},
		// banned holds editor on two-sum but viewer is explicitly denied.
		{"deny filters results", "user:banned", "viewer", nil},
		{"deny is relation-exact", "user:banned", "editor", []string{"two-sum"}},
		// contractor's only tuple is conditioned; empty context excludes it.
		{"conditioned allow excluded (empty context)", "user:contractor", "viewer", nil},
		// lena: author denied on binary-search, but editor (implied) unaffected.
		{"deny does not cascade", "user:lena", "editor", []string{"binary-search"}},
		{"unknown subject", "user:stranger", "viewer", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			subType, subID, _ := strings.Cut(tc.subject, ":")
			got, err := resolver.ListObjects(ctx, clientA, model, subType, subID, "problem", tc.relation)
			if err != nil {
				t.Fatalf("ListObjects returned error: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("ListObjects = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("ListObjects = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// TestListObjectsForwardReverseConsistency: every id returned by ListObjects
// passes Check with an empty context, and known-allowed ids appear.
func TestListObjectsForwardReverseConsistency(t *testing.T) {
	resolver, model := newFixture(t)
	ctx := context.Background()

	subjects := []string{"user:kush", "user:mina", "user:omar", "user:banned", "user:lena", "user:contractor", "user:temp", "user:stranger"}
	relations := []string{"author", "editor", "viewer"}
	allIDs := []string{"two-sum", "binary-search"}

	for _, subject := range subjects {
		subType, subID, _ := strings.Cut(subject, ":")
		for _, relation := range relations {
			listed, err := resolver.ListObjects(ctx, clientA, model, subType, subID, "problem", relation)
			if err != nil {
				t.Fatalf("ListObjects(%s, %s) error: %v", subject, relation, err)
			}
			inList := map[string]bool{}
			for _, id := range listed {
				inList[id] = true
			}
			// Forward and reverse must agree on every known object id.
			for _, id := range allIDs {
				allowed, reason, err := resolver.Check(ctx, clientA, model, subType, subID, relation, "problem", id, nil)
				if err != nil {
					t.Fatalf("Check(%s, %s, problem:%s) error: %v", subject, relation, id, err)
				}
				if allowed != inList[id] {
					t.Fatalf("forward/reverse mismatch for (%s, %s, problem:%s): Check=%v (%s), ListObjects contains=%v",
						subject, relation, id, allowed, reason, inList[id])
				}
			}
		}
	}
}
