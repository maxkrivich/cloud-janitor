# Plans

This directory contains proposed changes created by the `/plan` command.

## Workflow

1. **Create Plan**: Run `/plan <description>` to generate a detailed implementation plan
2. **Review**: Read the plan file and provide feedback or request changes
3. **Approve**: Say "Implement" to proceed with the implementation
4. **Track**: Completed plans serve as a record of changes made

## Plan File Format

Plans are named with date prefix for chronological ordering:

```
YYYY-MM-DD-short-description.md
```

Example: `2026-03-09-add-gcp-provider.md`

## Plan Structure

Each plan file contains:

- **Goal**: What the plan aims to achieve
- **Context**: Background information and reasoning
- **Changes**: Detailed list of files to create/modify
- **Implementation Steps**: Ordered tasks to complete
- **Testing**: How to verify the changes work
- **Status**: `proposed` | `approved` | `implemented` | `cancelled`

## Statuses

| Status | Meaning |
|--------|---------|
| `proposed` | Plan created, awaiting review |
| `approved` | User said "Implement", work in progress |
| `implemented` | All changes completed |
| `cancelled` | Plan abandoned or superseded |
