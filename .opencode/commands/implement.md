---
description: Implement a plan phase-by-phase with verification pauses
agent: tdd
---

# Implement Plan

You are tasked with implementing an approved technical plan from `.opencode/plans/`. Plans contain phases with specific changes and success criteria.

## Getting Started

**Plan file:** @$1

When given a plan path:
1. Read the plan completely
2. Check for any existing checkmarks (`- [x]`) indicating completed work
3. Read all files mentioned in the plan
4. **Read files fully** - never use limit/offset, you need complete context
5. Create a todo list to track your progress
6. Start implementing from the first unchecked item

If no plan path provided, ask for one:
```
Please provide a plan file to implement:
`/implement @.opencode/plans/YYYY-MM-DD-<description>.md`

Available plans:
[List files in .opencode/plans/]
```

## Implementation Philosophy

Plans are carefully designed, but reality can be messy. Your job is to:
- Follow the plan's intent while adapting to what you find
- Implement each phase fully before moving to the next
- Verify your work makes sense in the broader codebase context
- Update checkboxes in the plan as you complete items

When things don't match the plan exactly, think about why and communicate clearly.

## Phase Execution

For each phase:

### 1. Implement Changes
- Follow TDD: write tests first, then implementation
- Make the changes described in the plan
- Use existing patterns in the codebase

### 2. Run Automated Verification
Execute all automated checks from the plan's success criteria:
```bash
make test
make lint
make build
```

### 3. Update Plan Checkboxes
Use Edit to check off completed items in the plan file itself:
```markdown
- [x] Tests pass: `make test`
- [x] Linting passes: `make lint`
```

### 4. Pause for Manual Verification

After completing all automated verification for a phase, **STOP** and present:

```
## Phase [N] Complete - Ready for Manual Verification

**Automated verification passed:**
- [x] Tests pass: `make test`
- [x] Linting passes: `make lint`
- [x] Build succeeds: `make build`

**Please perform manual verification:**
- [ ] [Manual check from plan]
- [ ] [Another manual check]

Let me know when manual testing is complete so I can proceed to Phase [N+1].
```

**DO NOT proceed to the next phase until the user confirms manual verification is complete.**

## Handling Mismatches

If the plan doesn't match reality:

1. **STOP** and think about why
2. Present the issue clearly:

```
## Issue in Phase [N]

**Expected (from plan):** [what the plan says]
**Found:** [actual situation]
**Why this matters:** [explanation]

Options:
1. [How to adapt]
2. [Alternative approach]

How should I proceed?
```

3. Wait for guidance before continuing

## Resuming Partial Work

If the plan has existing checkmarks:
- Trust that completed work is done
- Pick up from the first unchecked item
- Verify previous work only if something seems off

To check progress:
```
## Resuming Implementation

**Completed phases:** [list checked phases]
**Current phase:** [first unchecked phase]
**Remaining:** [count of unchecked items]

Continuing from Phase [N]...
```

## Completion

When all phases are complete:

```
## Implementation Complete

All phases have been implemented and verified:
- [x] Phase 1: [name]
- [x] Phase 2: [name]
- [x] Phase 3: [name]

**Final verification:**
- [x] All tests pass: `make test`
- [x] Linting passes: `make lint`
- [x] Build succeeds: `make build`

The feature is ready for final review and PR creation.
```

## Quality Standards

Follow Cloud Janitor's coding standards:
- Use table-driven tests
- Wrap errors with context: `fmt.Errorf("context: %w", err)`
- Keep functions under 50 lines
- Document exported types and functions
- Follow Uber Go Style Guide

## Important Notes

- **Never skip manual verification pauses** - this is where human review catches issues
- **Update the plan file** as you complete items - this enables resuming
- **One phase at a time** - don't batch multiple phases
- **Read files fully** - partial reads lead to incomplete understanding
