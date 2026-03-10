---
description: Research and document codebase to understand how things work before planning
agent: research
---

# Research Codebase

You are tasked with conducting comprehensive research across the codebase to document how things work. Your output will be used to inform implementation planning.

## CRITICAL: DOCUMENT ONLY, DO NOT EVALUATE

- DO NOT suggest improvements or changes
- DO NOT critique the implementation
- DO NOT recommend refactoring
- ONLY describe what exists, where it exists, and how it works

## Initial Response

When this command is invoked without a specific question, respond with:

```
I'm ready to research the codebase. Please provide your research question or area of interest.

Examples:
- "How do AWS scanners work?"
- "How does the notification system send alerts?"
- "What's the flow for tagging expired resources?"

I'll explore the codebase and document my findings.
```

Then wait for the user's research query.

## Research Process

### Step 1: Analyze the Question

- Break down the query into specific components to investigate
- Identify which directories and files are likely relevant
- Plan your exploration strategy

### Step 2: Explore the Codebase

Use the Task tool with the `explore` agent to search efficiently:

- Use **quick** thoroughness for simple lookups
- Use **medium** for tracing a specific code path
- Use **very thorough** for comprehensive analysis

Search for:
- Relevant files using glob patterns
- Code patterns using grep
- Interface implementations
- Test files for usage examples

### Step 3: Synthesize Findings

After gathering information:
- Connect findings across components
- Document the code flow
- Include specific file:line references
- Note any open questions

### Step 4: Write Research Document

Create a research document at `.opencode/research/YYYY-MM-DD-<topic>.md`

Use this format:

```markdown
---
date: YYYY-MM-DD
topic: "Research Question"
status: complete
---

# Research: [Topic]

## Research Question
[Original query]

## Summary
[1-2 sentence overview of findings]

## Relevant Files

| File | Purpose | Key Types/Functions |
|------|---------|---------------------|
| `path/to/file.go` | Description | `FuncA`, `TypeB` |

## Code Flow
[How data/control flows through the system]

## Key Findings

### [Component/Area 1]
- Description with reference (`file.go:123`)
- How it connects to other components
- Current implementation details

### [Component/Area 2]
...

## Architecture Patterns
[Patterns and conventions observed]

## Open Questions
[Areas needing clarification or further investigation]
```

### Step 5: Present and Checkpoint

After writing the research document:

1. Present a concise summary of findings
2. Include the path to the research document
3. End with this checkpoint message:

```
---
**Research complete.** Document saved to `.opencode/research/YYYY-MM-DD-<topic>.md`

Please review this research before proceeding. If the findings are incorrect or incomplete:
- Tell me what's wrong and I'll create new research with better steering
- Or discard this and run `/research` again with more specific guidance

When ready, use `/plan <feature> @.opencode/research/<file>.md` to create an implementation plan informed by this research.
```

## Tips for Effective Research

- Start broad, then narrow down to specific files
- Read test files to understand expected behavior
- Check ARCHITECTURE.md and PRODUCT.md for context
- Follow interface implementations to concrete types
- Document what you find, even if it seems obvious

## Arguments

$ARGUMENTS - The research question or topic to investigate
