---
description: Create detailed implementation plans through interactive research and iteration
---

# Create Implementation Plan

You are tasked with creating detailed implementation plans through an interactive, iterative process. Be skeptical, thorough, and work collaboratively with the user to produce high-quality technical specifications.

## Initial Response

When this command is invoked:

1. **Check if parameters were provided**:
   - If a file path or research reference was provided, read it FULLY first
   - Begin the analysis process immediately

2. **If no parameters provided**, respond with:

```
I'll help you create a detailed implementation plan. Let me start by understanding what we're building.

Please provide:
1. The feature or task description
2. Any relevant context, constraints, or requirements
3. Reference to research if available: `@.opencode/research/<file>.md`

Tip: For complex features, run `/research <topic>` first to document the current codebase state.
```

Then wait for the user's input.

## Process Steps

### Step 1: Context Gathering & Initial Analysis

1. **Read all mentioned files FULLY**:
   - Research documents from `.opencode/research/`
   - Related plans from `.opencode/plans/`
   - Any referenced code files
   - **IMPORTANT**: Read files completely, not partially

2. **Explore the codebase** for relevant context:
   - Use Task tool with `explore` agent to find related files
   - Identify existing patterns to follow
   - Find similar implementations to model after

3. **Present your understanding and ask focused questions**:

```
Based on my research, I understand we need to [accurate summary].

I've found that:
- [Current implementation detail with file:line reference]
- [Relevant pattern or constraint discovered]
- [Potential complexity or edge case identified]

Questions I need answered:
- [Specific technical question requiring human judgment]
- [Business logic clarification]
- [Design preference that affects implementation]
```

Only ask questions you genuinely cannot answer through code investigation.

### Step 2: Research & Discovery

After getting initial clarifications:

1. **Verify any corrections**: If the user corrects a misunderstanding, explore the codebase to verify before proceeding.

2. **Use Task tool for deeper research** if needed:
   - Find specific implementation patterns
   - Understand how similar features were built
   - Identify integration points and dependencies

3. **Present findings and design options**:

```
Based on my research, here's what I found:

**Current State:**
- [Key discovery about existing code]
- [Pattern or convention to follow]

**Design Options:**
1. [Option A] - [pros/cons]
2. [Option B] - [pros/cons]

**Open Questions:**
- [Technical uncertainty]
- [Design decision needed]

Which approach aligns best with your vision?
```

### Step 3: Plan Structure Development

Once aligned on approach:

1. **Create initial plan outline**:

```
Here's my proposed plan structure:

## Overview
[1-2 sentence summary]

## Implementation Phases:
1. [Phase name] - [what it accomplishes]
2. [Phase name] - [what it accomplishes]
3. [Phase name] - [what it accomplishes]

Does this phasing make sense? Should I adjust the order or granularity?
```

2. **Get feedback on structure** before writing details

### Step 4: Detailed Plan Writing

After structure approval, write the plan to `.opencode/plans/YYYY-MM-DD-<description>.md`

Use this template:

````markdown
# [Feature Name] Implementation Plan

## Overview
[Brief description of what we're implementing and why]

## Current State Analysis
[What exists now, what's missing, key constraints discovered]

## Desired End State
[Specification of the end state and how to verify it]

## What We're NOT Doing
[Explicitly list out-of-scope items to prevent scope creep]

## Implementation Approach
[High-level strategy and reasoning]

---

## Phase 1: [Descriptive Name]

### Overview
[What this phase accomplishes]

### Changes Required

#### 1. [Component/File Group]
**File**: `path/to/file.go`
**Changes**: [Summary of changes]

```go
// Specific code to add/modify
```

### Success Criteria

#### Automated Verification:
- [ ] Tests pass: `make test`
- [ ] Linting passes: `make lint`
- [ ] Build succeeds: `make build`

#### Manual Verification:
- [ ] [Specific manual test step]
- [ ] [Another verification step]

**Implementation Note**: After completing this phase and all automated verification passes, pause for manual confirmation before proceeding to Phase 2.

---

## Phase 2: [Descriptive Name]
[Similar structure...]

---

## Testing Strategy

### Unit Tests
- [What to test]
- [Key edge cases]

### Integration Tests
- [End-to-end scenarios]

### Manual Testing Steps
1. [Specific step to verify feature]
2. [Edge case to test manually]

## References
- Research: `.opencode/research/[relevant].md`
- Similar implementation: `[file:line]`
````

### Step 5: Review and Iterate

1. **Present the draft plan location**:

```
I've created the implementation plan at:
`.opencode/plans/YYYY-MM-DD-<description>.md`

Please review it and let me know:
- Are the phases properly scoped?
- Are the success criteria specific enough?
- Any technical details that need adjustment?
- Missing edge cases or considerations?
```

2. **Iterate based on feedback** until the user is satisfied

3. **Final checkpoint**:

```
---
**Plan complete.** Saved to `.opencode/plans/YYYY-MM-DD-<description>.md`

When ready to implement, run:
`/implement @.opencode/plans/YYYY-MM-DD-<description>.md`

The implementation will proceed phase-by-phase with pauses for manual verification.
```

## Important Guidelines

### Be Skeptical
- Question vague requirements
- Identify potential issues early
- Don't assume - verify with code

### Be Interactive
- Don't write the full plan in one shot
- Get buy-in at each major step
- Allow course corrections

### Be Thorough
- Include specific file paths and line numbers
- Write measurable success criteria
- Separate automated vs manual verification
- Use `make` commands when possible

### Be Practical
- Focus on incremental, testable changes
- Consider migration and rollback
- Think about edge cases
- Include "what we're NOT doing"

### No Open Questions in Final Plan
- If you encounter open questions, STOP
- Research or ask for clarification immediately
- The plan must be complete and actionable
- Every decision must be made before finalizing

## Success Criteria Format

Always separate into two categories:

**Automated Verification** (can be run by agents):
- Commands: `make test`, `make lint`, `make build`
- Specific files that should exist
- Type checking / compilation

**Manual Verification** (requires human testing):
- Feature behavior verification
- Edge case handling
- User acceptance criteria

## Arguments

$ARGUMENTS - Feature request or task description. Can include `@.opencode/research/<file>.md` reference.
