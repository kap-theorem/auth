import { RelationTuple } from "../api";

/** A relation tuple rendered as a typographic fact: subject —relation→ object. */
export default function Fact({ tuple }: { tuple: RelationTuple }) {
  return (
    <span className={"fact" + (tuple.effect === "deny" ? " denied" : "")}>
      <span className="sub">
        {tuple.subject_type}:{tuple.subject_id}
      </span>
      <span className="rel">
        {" "}
        —{tuple.effect === "deny" ? "⊘" : ""}
        {tuple.relation}→{" "}
      </span>
      <span className="obj">
        {tuple.object_type}:{tuple.object_id}
      </span>
    </span>
  );
}
