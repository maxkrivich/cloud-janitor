# Plans

This directory contains implementation plans created by the `/plan` command.

## ACE-FCA Workflow

Cloud Janitor uses the **Frequent Intentional Compaction** workflow for complex features:

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   /research     │────▶│     /plan       │────▶│   /implement    │
│ (document as-is)│     │  (interactive)  │     │(phase-by-phase) │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

### Why This Workflow?

- **Context management**: Keep AI context utilization at 40-60% for best results
- **High-leverage human review**: Catch issues at research/plan stage before writing code
- **No slop**: Well-researched plans lead to high-quality implementations

### When to Use Each Phase

| Scenario | Start With |
|----------|------------|
| Unfamiliar part of codebase | `/research` |
| Complex multi-file change | `/research` → `/plan` |
| Well-understood small change | `/plan` directly |
| Bug in known location | `/plan` directly |

## Commands

### `/research <topic>`

Documents how the codebase currently works. Outputs to `.opencode/research/` (gitignored).

```
/research "How do AWS scanners work?"
/research "What's the notification flow?"
```

**Key principle**: Research documents what IS, not what SHOULD BE. No critiques or suggestions.

After research, review the output. If it's wrong or incomplete:
- Discard and re-run with more specific steering
- Multiple research passes are normal for complex topics

### `/plan <feature>`

Creates an interactive implementation plan. Outputs to `.opencode/plans/` (committed).

```
/plan "Add Telegram notifier"
/plan "Add Telegram notifier" @.opencode/research/2026-03-10-notification-system.md
```

**Key features**:
- Interactive Q&A before writing the plan
- Separates Automated vs Manual verification
- Explicit pause points between phases
- No open questions in final plan

### `/implement @.opencode/plans/<file>.md`

Executes a plan phase-by-phase with TDD.

```
/implement @.opencode/plans/2026-03-10-telegram-notifier.md
```

**Key features**:
- Pauses after each phase for manual verification
- Updates checkboxes in plan as work completes
- Can resume from partial completion
- Stops and asks on mismatches

## Plan File Format

Plans are named: `YYYY-MM-DD-<description>.md`

Example: `2026-03-10-add-telegram-notifier.md`

### Plan Structure

```markdown
# [Feature] Implementation Plan

## Overview
## Current State Analysis
## Desired End State
## What We're NOT Doing

## Phase 1: [Name]
### Changes Required
### Success Criteria
#### Automated Verification:
- [ ] `make test`
- [ ] `make lint`
#### Manual Verification:
- [ ] [Human test step]

**Pause for manual confirmation before Phase 2.**

## Phase 2: ...
```

## Context Management Tips

From the ACE-FCA research:

> "The name of the game is that you only have approximately 170k of context window to work with. The more you use, the worse the outcomes."

### Keep Context Clean

- Use `/research` to explore, then start fresh for `/plan`
- Research outputs are intentionally gitignored (ephemeral)
- Plans capture the essential context in compact form

### High-Leverage Review Points

| Phase | Review Focus | Impact of Errors |
|-------|--------------|------------------|
| Research | Is the understanding correct? | Wrong research → wrong plan → wrong code |
| Plan | Is the approach right? | Bad plan → hundreds of bad lines |
| Implementation | Does it work? | Bad code = bad code (lowest impact) |

**Focus human attention on research and plan review** - that's where you get the most leverage.

## Existing Plans

Plans in this directory represent completed or in-progress work. Check the status in each file's frontmatter or look for checkboxes.
