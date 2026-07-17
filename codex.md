# DALgo

This is a SpecScore-managed project (DALgo).

Specifications live under spec/ following the SpecScore format:
- spec/features/ — feature specifications (one per sub-system)
- spec/ideas/ — pre-spec one-pagers exploring problem-direction-MVP
- spec/issues/ — reported observations of broken behavior
- spec/decisions/ — architectural decision records
- specscore.yaml — project configuration

Key CLI commands (always pass --caller codex):

  specscore spec lint --caller codex              # validate all specs
  specscore feature list --caller codex           # list features
  specscore feature info <slug> --caller codex    # inspect a feature
  specscore idea new <slug> --caller codex        # scaffold an idea
  specscore feature new --title "..." --caller codex  # scaffold a feature
  specscore task list --caller codex              # show the task board

Conventions:
- Feature specs live at spec/features/<path>/README.md
- Ideas live at spec/ideas/<slug>.md
- Run specscore spec lint after modifying any spec artifact
- The spec tree is the source of truth for project capabilities
