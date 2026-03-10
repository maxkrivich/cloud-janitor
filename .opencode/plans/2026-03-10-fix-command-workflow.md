# Fix Command Workflow Implementation Plan

## Overview
Fix the `/research` → `/plan` → `/implement` workflow to ensure proper context preservation and document creation. The current issue is that the `agent:` directive in `research.md` spawns a subagent that loses conversation context, and commands lack explicit guardrails against premature implementation.

## Current State Analysis

### Problem 1: Context Loss in `/research`
- **File**: `.opencode/commands/research.md:3`
- **Issue**: `agent: research` spawns a fresh subagent that loses conversation context
- **Impact**: Research findings aren't connected to the original conversation; user has to re-explain context

### Problem 2: Missing Explicit Guardrails
- **File**: `.opencode/commands/research.md`
- **Issue**: No explicit "DO NOT IMPLEMENT" warning at the end
- **Impact**: Risk of accidentally proceeding to implementation

### Problem 3: Implement Command Context
- **File**: `.opencode/commands/implement.md:3`
- **Issue**: `agent: tdd` spawns a subagent, which is intentional for clean context but may lose plan context
- **Impact**: Less critical since plans are self-contained documents

### What Works Well
- `plan.md` has no `agent:` directive - runs in main conversation (keeps context)
- `plan.md` has excellent "DO NOT IMPLEMENT" warnings (lines 198, 223-227, 232-237)
- Document templates are well-structured
- Checkpoint messages are clear

## Desired End State

1. `/research` runs in main conversation, preserving context
2. All commands have explicit "STOP" markers to prevent premature implementation
3. Document creation is mandatory before proceeding
4. Clear workflow: research → plan → implement with human review at each step

## What We're NOT Doing

- Not changing the `implement.md` agent directive (subagent is appropriate for clean execution context)
- Not restructuring the document templates (they're already good)
- Not adding new commands or workflows

## Implementation Approach

Remove the `agent: research` directive and add explicit guardrails to prevent workflow shortcuts.

---

## Phase 1: Fix Research Command

### Overview
Remove the `agent:` directive from `research.md` so it runs in the main conversation, preserving context. Add explicit "DO NOT IMPLEMENT" warnings.

### Changes Required

#### 1. Remove agent directive
**File**: `.opencode/commands/research.md`
**Change**: Remove line 3 (`agent: research`)

Current (lines 1-4):
```yaml
---
description: Research and document codebase to understand how things work before planning
agent: research
---
```

New:
```yaml
---
description: Research and document codebase to understand how things work before planning
---
```

#### 2. Add explicit stop warning at end
**File**: `.opencode/commands/research.md`
**Change**: Add warning after the final checkpoint message (after line 128)

Add before `## Tips for Effective Research`:
```markdown
## CRITICAL: Do Not Skip Steps

- **DO NOT proceed to implementation** after research
- **DO NOT create a plan** without user review of research
- **STOP** after presenting findings and wait for user direction
- The workflow is: `/research` → user review → `/plan` → user review → `/implement`
```

### Success Criteria

#### Automated Verification:
- [x] File syntax is valid YAML frontmatter
- [x] No `agent:` directive in frontmatter

#### Manual Verification:
- [ ] Run `/research` and verify it stays in main conversation (no context loss)
- [ ] Verify research document is created before proceeding
- [ ] Verify command stops and waits for user after research

**Implementation Note**: After completing this phase and all verification passes, pause for manual confirmation before proceeding to Phase 2.

---

## Phase 2: Strengthen Plan Command Guardrails

### Overview
While `plan.md` already has good guardrails, reinforce them by adding a critical warning at the very top of the process section.

### Changes Required

#### 1. Add early warning in process section
**File**: `.opencode/commands/plan.md`
**Change**: Add warning after line 32 (before "### Step 1")

Add:
```markdown
## CRITICAL REMINDER

**This command creates plans only. It does NOT implement them.**
- After the plan is written, you MUST STOP
- Wait for the user to run `/implement` explicitly
- "Continue" or "proceed" means iterate on the plan, NOT implement it
```

### Success Criteria

#### Automated Verification:
- [x] File syntax is valid YAML frontmatter

#### Manual Verification:
- [ ] Run `/plan` and verify it creates document without implementing
- [ ] Verify checkpoint message appears after plan creation
- [ ] Verify saying "continue" prompts for clarification, not implementation

**Implementation Note**: After completing this phase and all verification passes, pause for manual confirmation before proceeding to Phase 3.

---

## Phase 3: Update Implement Command Documentation

### Overview
Add clarity to `implement.md` about why it uses a subagent and what context it needs.

### Changes Required

#### 1. Add context explanation
**File**: `.opencode/commands/implement.md`
**Change**: Add explanation after line 8

Add after "Plans contain phases with specific changes and success criteria.":
```markdown

**Note**: This command uses `agent: tdd` to spawn a fresh context. This is intentional:
- Implementation should be based solely on the approved plan document
- The plan file contains all necessary context and specifications
- This prevents scope creep from conversation context
```

### Success Criteria

#### Automated Verification:
- [x] File syntax is valid YAML frontmatter

#### Manual Verification:
- [ ] Documentation is clear about why subagent is used

---

## Testing Strategy

### Manual Testing Steps

1. **Test Research Flow**:
   ```
   /research How do AWS scanners work?
   ```
   - Verify: Runs in main conversation
   - Verify: Creates `.opencode/research/2026-03-10-aws-scanners.md`
   - Verify: Stops and asks for review

2. **Test Plan Flow**:
   ```
   /plan Add a new scanner @.opencode/research/2026-03-10-aws-scanners.md
   ```
   - Verify: Reads research document
   - Verify: Creates plan document
   - Verify: Does NOT start implementing
   - Verify: Shows "DO NOT IMPLEMENT" warning

3. **Test Implement Flow**:
   ```
   /implement @.opencode/plans/2026-03-10-new-scanner.md
   ```
   - Verify: Reads plan fully
   - Verify: Implements phase by phase
   - Verify: Pauses for manual verification

## References
- HumanLayer commands: https://github.com/humanlayer/humanlayer/tree/main/.claude/commands
- ACE Article on Context Engineering: https://www.anthropic.com/engineering/claude-context-engineering

---

> **DO NOT IMPLEMENT** - This plan requires explicit `/implement` command to begin execution.
