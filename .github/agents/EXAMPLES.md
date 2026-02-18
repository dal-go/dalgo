# GitHub Agents Usage Examples

This document provides examples of how to use the inGitDB GitHub Agents in various scenarios.

## Using Agents in GitHub Copilot

When using GitHub Copilot, you can reference these agents using the `@` symbol:

```
@go-coder please write unit tests for the validator package

@go-engineer implement the new query command with proper error handling

@docs-writer update the README with the new query command documentation

@schema-designer create a schema for storing user profiles with authentication data
```

## Example Workflows

### 1. Implementing a New Feature

```
Step 1: @spec-writer create a specification for the new "query" command
Step 2: @go-engineer implement the query command based on the spec
Step 3: @go-tester write comprehensive tests for the query command
Step 4: @ci-runner run the full quality gate
Step 5: @go-reviewer review all changes
Step 6: @docs-writer update documentation for the new command
```

### 2. Fixing a Bug

```
Step 1: @go-fixer investigate and fix the nil pointer panic in validator
Step 2: @go-tester add regression tests for the bug
Step 3: @ci-runner verify the fix passes all gates
Step 4: @go-reviewer ensure code quality
```

### 3. Writing Documentation

```
Step 1: @docs-writer create component documentation for the new MCP server
Step 2: @docs-writer update ARCHITECTURE.md with the new component
Step 3: @docs-writer add examples to the README
```

### 4. Schema Design

```
Step 1: @schema-designer create schemas for a multi-tenant task management system
Step 2: @go-coder implement schema validation logic
Step 3: @go-tester write tests for the schema validation
Step 4: @docs-writer document the schema structure
```

## Agent Collaboration Patterns

### Pattern 1: Spec → Implementation → Test → Review

Best for: New features, significant changes

```
@spec-writer → @go-engineer → @go-tester → @go-reviewer → @ci-runner
```

### Pattern 2: Quick Fix

Best for: Simple bugs, lint errors, small changes

```
@go-coder → @ci-runner → @go-reviewer
```

### Pattern 3: Debugging

Best for: Complex bugs, performance issues, race conditions

```
@go-fixer → @go-tester (regression tests) → @ci-runner
```

### Pattern 4: Documentation

Best for: Docs updates, new documentation

```
@docs-writer → (optional: @go-reviewer for code examples)
```

## GitHub Actions Integration

While these agents are primarily designed for interactive use with GitHub Copilot, they can inform GitHub Actions workflow design:

```yaml
name: Development Workflow

on: [pull_request]

jobs:
  # Inspired by go-coder and ci-runner agents
  build-and-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      
      # Following ci-runner agent's quality gates
      - name: Build
        run: go build ./...
      
      - name: Test
        run: go test -timeout=10s ./...
      
      - name: Lint
        run: golangci-lint run

  # Inspired by go-reviewer agent
  code-review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Review conventions
        run: |
          # Check for nested calls
          # Check error handling
          # etc.
```

## Model Selection Guide

Choose the appropriate agent based on task complexity:

| Task Complexity | Agent | Model |
|----------------|-------|-------|
| Simple, well-defined | go-coder, ci-runner, go-reviewer | haiku (fast) |
| Medium complexity | go-tester, go-fixer, docs-writer, schema-designer, spec-writer | sonnet (balanced) |
| Complex, requires deep reasoning | go-engineer | opus (powerful) |

## Tips for Effective Agent Usage

1. **Be Specific**: Provide clear context and requirements
   - ❌ "Fix the tests"
   - ✅ "@go-fixer investigate and fix the TestValidate_MissingField test failure"

2. **Use the Right Agent**: Match the agent to the task
   - Small fix → @go-coder
   - Large refactor → @go-engineer
   - Documentation → @docs-writer

3. **Follow Workflows**: Use agent collaboration patterns
   - Always run @ci-runner before considering a task complete
   - Use @go-reviewer to catch issues early

4. **Leverage Agent Knowledge**: Each agent knows the codebase conventions
   - They follow CLAUDE.md rules automatically
   - They understand the project architecture
   - They use the right tools and patterns

5. **Iterate**: If an agent's first attempt isn't perfect, provide feedback
   - "@go-coder the test you wrote doesn't cover the error case when path is empty"
   - "@docs-writer add more examples to the README"

## Example Prompts

### For go-coder
```
@go-coder implement the expandHome function in cmd/ingitdb/commands/validate.go
that expands ~ to the user's home directory, following CLAUDE.md conventions
```

### For go-engineer
```
@go-engineer refactor the validator package to support incremental validation
based on git diffs. This needs to touch pkg/ingitdb/validator/ and potentially
add new interfaces. Consider caching validated files.
```

### For go-tester
```
@go-tester write comprehensive tests for pkg/ingitdb/validator/data_validator.go
including happy paths, error cases, and edge cases like empty files, invalid YAML,
and missing required fields
```

### For go-fixer
```
@go-fixer the TestValidate test is failing with a nil pointer panic when
processing files with empty collections. Investigate and fix with proper
error handling. Add regression test.
```

### For docs-writer
```
@docs-writer create a new document docs/components/incremental-validation.md
explaining how the incremental validation feature works, when to use it,
and provide CLI examples
```

### For schema-designer
```
@schema-designer design a schema for storing GitHub repository metadata
including: repo name, owner, stars, forks, last_updated, and topics.
Use appropriate foreign keys and constraints.
```

### For spec-writer
```
@spec-writer create a BACKLOG entry for implementing the "ingitdb query"
command that allows querying records by field values with support for
operators like equals, contains, greater-than, etc.
```

### For go-reviewer
```
@go-reviewer review the changes in cmd/ingitdb/commands/query.go against
CLAUDE.md conventions and check if tests are comprehensive
```

### For ci-runner
```
@ci-runner run the full quality gate (build, test, lint) and report results
```

## Troubleshooting

**Agent not found?**
- Ensure you're using the correct agent name (lowercase, hyphenated)
- Check that you're in a repository with `.github/agents/` directory

**Agent gives unexpected results?**
- Provide more context in your prompt
- Specify exact files and functions
- Reference specific conventions or patterns to follow

**Need a different agent?**
- Check the agent descriptions in README.md
- Consider the model (haiku/sonnet/opus) for your task complexity
- Use agent collaboration patterns for multi-step tasks

## References

- [GitHub Agents README](README.md) - Overview and agent descriptions
- [Claude Agents (original)](.claude/agents/) - Source definitions
- [GitHub Copilot Documentation](https://docs.github.com/en/copilot)
- [Project CLAUDE.md](../../CLAUDE.md) - Coding conventions
