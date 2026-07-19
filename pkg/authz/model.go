// Package authz implements the Phase 3 authorization engine (design spec
// §6): the authz model schema, the relation-tuple Check resolver
// (implications, userset hops, deny-wins, conditions, depth limit), and the
// ABAC condition expression evaluator.
package authz

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Authz model JSON schema
// -----------------------
//
// The model declares an app's authorization vocabulary: its object types,
// each type's relations, and which relations imply which. The shape is the
// one the console already uses (console/src/api/mock.ts):
//
//	{
//	  "types": {
//	    "<object_type>": {
//	      "relations": {
//	        "<relation>": ["<relations that imply it>", ...]
//	      }
//	    }
//	  }
//	}
//
// Example — "author implies editor implies viewer":
//
//	{
//	  "types": {
//	    "problem": {
//	      "relations": {
//	        "author": [],
//	        "editor": ["author"],
//	        "viewer": ["editor"]
//	      }
//	    }
//	  }
//	}
//
// A relation's list names the *stronger* relations whose grant also
// satisfies it. Checking "viewer" therefore accepts author/editor/viewer
// tuples. Validation rules enforced by ParseModel:
//
//   - the document must be a JSON object with only the keys shown above
//     (unknown keys are rejected — they are almost always typos);
//   - type and relation names must be non-empty;
//   - every relation named in an implication list must be declared on the
//     same type;
//   - the implication graph of each type must be acyclic (self-implication
//     included).
//
// Relations used as usersets (subject_type='role') need no special
// declaration: role membership is expressed with ordinary tuples on objects
// of type "role" (relation "member"), and the "role" type may itself appear
// in the model to give its relations implications.

// Model is a parsed, validated authz model.
type Model struct {
	Types map[string]TypeDef `json:"types"`
}

// TypeDef declares one object type's relations. Each relation maps to the
// list of relations that imply it.
type TypeDef struct {
	Relations map[string][]string `json:"relations"`
}

// ParseModel parses and validates model JSON against the schema documented
// above. It returns a descriptive error on any schema violation.
func ParseModel(modelJSON string) (*Model, error) {
	dec := json.NewDecoder(bytes.NewReader([]byte(modelJSON)))
	dec.DisallowUnknownFields()
	var m Model
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("model must be a JSON object of the form {\"types\": {...}}: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("model contains trailing data after the JSON object")
	}

	for typeName, td := range m.Types {
		if typeName == "" {
			return nil, fmt.Errorf("object type names must be non-empty")
		}
		for relation, impliers := range td.Relations {
			if relation == "" {
				return nil, fmt.Errorf("type %q: relation names must be non-empty", typeName)
			}
			for _, implier := range impliers {
				if _, ok := td.Relations[implier]; !ok {
					return nil, fmt.Errorf("type %q: relation %q lists unknown relation %q as an implier", typeName, relation, implier)
				}
			}
		}
		if cycle := findImplicationCycle(td.Relations); cycle != "" {
			return nil, fmt.Errorf("type %q: implication cycle through relation %q", typeName, cycle)
		}
	}
	return &m, nil
}

// findImplicationCycle returns a relation on a cycle in the implication
// graph, or "" if the graph is acyclic. Edges run relation → its impliers.
func findImplicationCycle(relations map[string][]string) string {
	const (
		unvisited = 0
		inStack   = 1
		done      = 2
	)
	state := make(map[string]int, len(relations))
	var visit func(rel string) string
	visit = func(rel string) string {
		switch state[rel] {
		case inStack:
			return rel
		case done:
			return ""
		}
		state[rel] = inStack
		for _, implier := range relations[rel] {
			if hit := visit(implier); hit != "" {
				return hit
			}
		}
		state[rel] = done
		return ""
	}
	for rel := range relations {
		if hit := visit(rel); hit != "" {
			return hit
		}
	}
	return ""
}

// Expand returns the set of relations whose grant satisfies `relation` on
// `objectType` (always including the relation itself), following the
// model's implication graph transitively. Safe on a nil Model (no model
// written yet): only the relation itself is returned — identical to the
// console mock's behavior.
func (m *Model) Expand(objectType, relation string) map[string]bool {
	out := map[string]bool{relation: true}
	if m == nil {
		return out
	}
	td, ok := m.Types[objectType]
	if !ok {
		return out
	}
	queue := []string{relation}
	for len(queue) > 0 {
		rel := queue[0]
		queue = queue[1:]
		for _, implier := range td.Relations[rel] {
			if !out[implier] {
				out[implier] = true
				queue = append(queue, implier)
			}
		}
	}
	return out
}
