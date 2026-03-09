---
description: Implement a feature from an existing plan file
agent: tdd
---

Implement the feature described in the plan using test-driven development.

**Plan file:** @$1

## Instructions

1. Read the implementation plan carefully
2. For each task in the plan:
   - Write tests first
   - Implement the code
   - Verify tests pass
   - Move to the next task
3. Run the full test suite before completing
4. Report any deviations from the plan

## Quality Gates

Before marking complete:
- [ ] All tests pass (`make test`)
- [ ] Linter passes (`make lint`)
- [ ] Code compiles (`make build`)
- [ ] All planned tasks implemented
