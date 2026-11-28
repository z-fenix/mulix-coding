---
name: setup-mulix
description: Use when installing or reconfiguring Mulix for a project in Claude Code or DeepSeek Harness.
---

# Set up Mulix

This skill is distributed separately from the binary. Run `mulix version` and ensure it matches the source release that supplied this skill; stop and ask if the version cannot be matched. Use the user's explicit destination, agent hosts (`claude`, `dsh`, or both), and tracker (`markdown`, `github`, or `gitlab`); ask for any missing choices. For remote trackers require the exact hostname and project path, including GitLab subgroups. Never infer a repository or host from local defaults; credentials remain in `gh` or `glab`.

Before invoking the bridge, inspect any existing `CLAUDE.md`, `AGENTS.md`, and `docs/agents/{issue-tracker,triage-labels,domain}.md` in the destination for tracker, label, and domain rules. If the selected configuration would contradict a preserved rule, stop without writing and ask the user to reconcile the conflict. Preserve unrelated context. Unowned agent documents or edited managed sections also require manual reconciliation when the bridge reports them.

For interactive setup, present the full selection as a draft and obtain confirmation before writing. Complete explicit noninteractive choices use the same bridge and validation. For an existing `.mulix/tracker.json`, show the current target and obtain an explicit choice before changing it. Probe access only reads the project; it does not authorize remote changes. If a probe fails, ask whether to proceed offline; only if the user agrees add `--allow-pending`. A new target stays pending; a previously verified same target keeps its ID but its live recheck failed. Otherwise stop. Do not create remote issues or labels.

After confirmation call the hidden bridge with explicit arguments. For example:

```sh
mulix _setup --dir /path/to/project --host claude,dsh --tracker gitlab --hostname git.example.com --project group/subgroup/project
```

For Markdown omit `--hostname` and `--project`. Use `--force` only when the user requests replacement of managed files; review backup and conflict messages. Preserve user instructions and resolve any reported conflicts manually. Re-running the same selection is safe. Report pending verification and any skipped or conflicting files.
