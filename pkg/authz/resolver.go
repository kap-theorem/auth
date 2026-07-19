package authz

import (
	"authservice/pkg/models"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// MaxDepth bounds userset (role) recursion. Exceeding it denies the whole
// check with an explanatory reason (fail closed).
const MaxDepth = 20

// Well-known tuple vocabulary for usersets: a tuple whose subject is
// role:R grants through R's membership, and membership itself is ordinary
// tuples on the object role:R with the relation "member" — identical to the
// console mock's semantics (console/src/api/mock.ts), so mock and real
// resolvers agree.
const (
	RoleSubjectType    = "role"
	RoleObjectType     = "role"
	RoleMemberRelation = "member"
)

var errDepthExceeded = errors.New("recursion depth limit exceeded")

// TupleStore is the read surface the resolver needs. Both queries are
// served by the tuple table's primary key / subject index.
type TupleStore interface {
	// TuplesForObject returns every tuple (any relation, any effect) on one
	// object within a client scope.
	TuplesForObject(ctx context.Context, clientID, objectType, objectID string) ([]models.RelationTuple, error)
	// TuplesBySubject returns every tuple within a client scope whose
	// subject matches, restricted to one object type (the subject index).
	TuplesBySubject(ctx context.Context, clientID, subjectType, subjectID, objectType string) ([]models.RelationTuple, error)
}

// Resolver is the full Phase 3 Check engine (design spec §6).
type Resolver struct {
	store TupleStore
}

func NewResolver(store TupleStore) *Resolver {
	return &Resolver{store: store}
}

// Check resolves whether subject has relation on object within a client
// scope, per spec §6:
//
//  1. Deny pass first, with full expansion of the subject side (userset
//     hops) but the EXACT relation only — deny does not travel through
//     implications. Any match → denied, stop.
//  2. Allow pass: the requested relation expands through the model's
//     implication graph; each candidate tuple matches the subject directly
//     or via a role (userset) hop into role membership.
//  3. Conditions: a conditioned tuple only matches when its condition
//     evaluates true against condCtx (fail closed on parse errors and
//     missing keys) — so a conditioned deny that evaluates true denies,
//     and a false condition makes the tuple invisible to either pass.
//  4. Default deny. Recursion is bounded by MaxDepth; exceeding it denies
//     with an explanatory reason.
//
// model may be nil (no model written): only exact-relation matches apply.
func (r *Resolver) Check(ctx context.Context, clientID string, model *Model, subjectType, subjectID, relation, objectType, objectID string, condCtx map[string]string) (bool, string, error) {
	allowed, reason, err := r.resolve(ctx, clientID, model, subjectType, subjectID, relation, objectType, objectID, condCtx, 0)
	if errors.Is(err, errDepthExceeded) {
		return false, fmt.Sprintf("recursion depth limit (%d) exceeded (denying)", MaxDepth), nil
	}
	return allowed, reason, err
}

func (r *Resolver) resolve(ctx context.Context, clientID string, model *Model, subjectType, subjectID, relation, objectType, objectID string, condCtx map[string]string, depth int) (bool, string, error) {
	if depth > MaxDepth {
		return false, "", errDepthExceeded
	}

	onObject, err := r.store.TuplesForObject(ctx, clientID, objectType, objectID)
	if err != nil {
		return false, "", err
	}

	// matches reports whether a tuple's subject side reaches the checked
	// subject: condition gate, then direct match, then userset (role) hop.
	matches := func(t models.RelationTuple) (bool, string, error) {
		if strings.TrimSpace(t.ConditionExpr) != "" && !EvalCondition(t.ConditionExpr, condCtx) {
			return false, "", nil // failed/unparseable condition → tuple doesn't match
		}
		if t.SubjectType == subjectType && t.SubjectID == subjectID {
			return true, fmt.Sprintf("matched tuple %s", fmtTuple(t)), nil
		}
		if t.SubjectType == RoleSubjectType {
			// Userset hop: is the subject a member of this role?
			hopOK, _, hopErr := r.resolve(ctx, clientID, model, subjectType, subjectID, RoleMemberRelation, RoleObjectType, t.SubjectID, condCtx, depth+1)
			if hopErr != nil {
				return false, "", hopErr
			}
			if hopOK {
				return true, fmt.Sprintf("via role:%s → %s", t.SubjectID, fmtTuple(t)), nil
			}
		}
		return false, "", nil
	}

	// 1. Deny pass — exact relation only, deny always wins.
	for _, t := range onObject {
		if t.Effect != models.EffectDeny || t.Relation != relation {
			continue
		}
		hit, _, err := matches(t)
		if err != nil {
			return false, "", err
		}
		if hit {
			return false, fmt.Sprintf("explicit deny: %s", fmtTuple(t)), nil
		}
	}

	// 2. Allow pass — expand the relation through the model's implications.
	expanded := model.Expand(objectType, relation)
	for _, t := range onObject {
		if t.Effect != models.EffectAllow || !expanded[t.Relation] {
			continue
		}
		hit, why, err := matches(t)
		if err != nil {
			return false, "", err
		}
		if hit {
			return true, why, nil
		}
	}

	// 3. Default deny.
	return false, "no matching tuple (default deny)", nil
}

// ListObjects returns every object id of objectType the subject can reach
// for relation (spec §6 — "everything user X can edit", for UIs). It walks
// the subject index in reverse: first the roles the subject can reach
// (nested usersets included), then the candidate objects any of those
// subjects appear on; each candidate is then confirmed with a full Check so
// forward and reverse resolution can never disagree. Conditions are
// evaluated with an EMPTY context, so conditioned allow tuples are excluded
// from the results.
func (r *Resolver) ListObjects(ctx context.Context, clientID string, model *Model, subjectType, subjectID, objectType, relation string) ([]string, error) {
	// Roles reachable from the subject (over-approximation: membership is
	// confirmed by the final Check, which also applies deny tuples,
	// conditions, and the depth limit).
	type ref struct{ typ, id string }
	subjects := []ref{{subjectType, subjectID}}
	seenRoles := map[string]bool{}
	frontier := []ref{{subjectType, subjectID}}
	for len(frontier) > 0 {
		cur := frontier[0]
		frontier = frontier[1:]
		roleTuples, err := r.store.TuplesBySubject(ctx, clientID, cur.typ, cur.id, RoleObjectType)
		if err != nil {
			return nil, err
		}
		for _, t := range roleTuples {
			if !seenRoles[t.ObjectID] {
				seenRoles[t.ObjectID] = true
				role := ref{RoleSubjectType, t.ObjectID}
				subjects = append(subjects, role)
				frontier = append(frontier, role)
			}
		}
	}

	// Candidate objects: anything of the target type that carries an allow
	// tuple for the subject or one of its reachable roles.
	candidates := map[string]bool{}
	for _, s := range subjects {
		tuples, err := r.store.TuplesBySubject(ctx, clientID, s.typ, s.id, objectType)
		if err != nil {
			return nil, err
		}
		for _, t := range tuples {
			if t.Effect == models.EffectAllow {
				candidates[t.ObjectID] = true
			}
		}
	}

	// Confirm each candidate with the forward resolver (empty context).
	out := make([]string, 0, len(candidates))
	for id := range candidates {
		allowed, _, err := r.Check(ctx, clientID, model, subjectType, subjectID, relation, objectType, id, nil)
		if err != nil {
			return nil, err
		}
		if allowed {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out, nil
}

func fmtTuple(t models.RelationTuple) string {
	return fmt.Sprintf("%s:%s —%s→ %s:%s", t.SubjectType, t.SubjectID, t.Relation, t.ObjectType, t.ObjectID)
}
