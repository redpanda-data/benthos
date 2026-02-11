# 11. Execution Model

> ⚠️ **Note**: The dual execution model (Mapping vs Mutation) described below is under review as part of V2 development (see Solution 3 in PROPOSED_SOLUTIONS.md). This section documents current behavior but may change in the final V2 release. The goal is to unify around a single, predictable execution model.

## 11.1 Mapping Processor (Immutable)

Creates entirely new output document. Input document (`input`) remains immutable throughout execution:
```bloblang
output.id = input.id
output.invitees = input.invitees.filter(i -> i.mood >= 0.5)
output.rejected = input.invitees.filter(i -> i.mood < 0.5)  # Original still accessible
```

**Use Case**: Output shape significantly differs from input.

## 11.2 Mutation Processor (Mutable)

Directly modifies input document. Input document changes during execution:
```bloblang
output.rejected = input.invitees.filter(i -> i.mood < 0.5)  # Copy before mutation
output.invitees = input.invitees.filter(i -> i.mood >= 0.5) # Mutates original
```

**Use Case**: Output shape similar to input; avoids data copying overhead.

**Caution**: Assignment order matters; later assignments see mutated state.

## 11.3 Evaluation Order

Assignments execute sequentially in source order. Variables must be declared before use.
