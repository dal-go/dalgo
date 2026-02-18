# GitHub Agents for inGitDB

This directory contains agent definitions for GitHub Copilot that can be used in GitHub Actions and other GitHub Agents-compatible environments.

## Available Agents

### Development Agents

1. **go-coder** (`haiku` model)
   - Self-contained Go coding tasks
   - Writing unit tests for isolated functions
   - Implementing small well-specified features
   - Fixing simple lint errors

2. **go-engineer** (`opus` model)
   - Complex Go engineering tasks
   - Large refactors spanning multiple packages
   - Architectural decisions
   - Performance work

3. **go-tester** (`sonnet` model)
   - Comprehensive test suite writing
   - Table-driven tests, edge cases, error paths
   - Property-based tests and benchmarks
   - Test coverage analysis

4. **go-fixer** (`sonnet` model)
   - Bug investigation and fixing
   - Test failures, panics, race conditions
   - Memory leaks and performance regressions
   - Systematic diagnosis with instrumentation

5. **go-reviewer** (`haiku` model)
   - Code review for Go changes
   - Convention violations checking
   - Test completeness verification
   - API design review

### CI/CD Agent

6. **ci-runner** (`haiku` model)
   - Full quality gate execution (build, test, lint)
   - Structured pass/fail reporting
   - Standard final check before task completion

### Documentation Agents

7. **docs-writer** (`sonnet` model)
   - Feature documentation, component docs
   - README, ROADMAP, BACKLOG updates
   - Competitor analysis
   - Documentation verification against code

8. **spec-writer** (`sonnet` model)
   - Turn feature ideas into structured specifications
   - Create BACKLOG.md entries
   - Define acceptance criteria and test cases
   - Implementation notes and constraints

### Schema Agent

9. **schema-designer** (`sonnet` model)
   - Design inGitDB schemas
   - Create `.ingitdb.yaml` configurations
   - Define collection and view schemas
   - Schema validation and review

## Agent Format

Each agent definition follows the GitHub Agents format with YAML frontmatter:

```yaml
---
name: agent-name
description: >
  Brief description of what the agent does
target: github-copilot
tools:
  - read
  - write
  - edit
  - bash
model: haiku|sonnet|opus
---

# Agent instructions in Markdown format
```

### Key Fields

- **name**: Unique identifier for the agent
- **description**: Brief explanation of the agent's purpose and when to use it
- **target**: Set to `github-copilot` for GitHub Agents compatibility
- **tools**: Array of lowercase tool names the agent can access
- **model**: Recommended AI model (`haiku`, `sonnet`, or `opus`)

### Available Tools

- `read`: Read files from the repository
- `write`: Create new files
- `edit`: Modify existing files
- `glob`: Pattern-based file searching
- `grep`: Search file contents
- `bash`: Execute shell commands
- `task`: Delegate to sub-agents (for go-engineer)
- `websearch`: Web search capability (for docs-writer, spec-writer)
- `webfetch`: Fetch web pages (for docs-writer, spec-writer)

## Usage in GitHub Actions

These agent definitions can be referenced in GitHub Actions workflows and used with GitHub Copilot's agent system. The agents are designed to work together:

- **go-engineer** can delegate simple tasks to **go-coder**
- All coding agents should use **ci-runner** for final verification
- **go-reviewer** should check all code changes before completion
- **spec-writer** creates specifications that **go-engineer** implements
- **docs-writer** keeps documentation in sync with code changes

## Relationship to Claude Agents

These GitHub Agents were automatically converted from the original Claude agent definitions in `.claude/agents/`. The core instructions and expertise remain the same, but the format has been adapted for GitHub Agents compatibility.

Key differences:
- Added `target: github-copilot` for GitHub integration
- Tool names converted to lowercase array format
- YAML structure follows GitHub Agents schema
- All original instructions and domain knowledge preserved

## Contributing

When adding or updating agents:

1. Follow the GitHub Agents YAML schema
2. Keep tool names lowercase
3. Set `target: github-copilot`
4. Include comprehensive instructions in the Markdown body
5. Maintain consistency with existing agents
6. Update this README to reflect new agents

## Validation

To validate agent definitions:

```python
python3 << 'EOF'
import yaml
import sys

with open('.github/agents/your-agent.md', 'r') as f:
    content = f.read()
    parts = content.split('---\n', 2)
    frontmatter = yaml.safe_load(parts[1])
    
    required = ['name', 'description', 'target', 'tools', 'model']
    for field in required:
        assert field in frontmatter, f"Missing {field}"
    assert frontmatter['target'] == 'github-copilot'
    assert isinstance(frontmatter['tools'], list)
    print("✓ Valid agent definition")
EOF
```

## References

- [GitHub Copilot Custom Agents Documentation](https://docs.github.com/en/copilot/reference/custom-agents-configuration)
- [AgentSchema Specification](https://microsoft.github.io/AgentSchema/)
- [Project CLAUDE.md](../../CLAUDE.md) - Project conventions
- [Original Claude Agents](../../.claude/agents/) - Source definitions
