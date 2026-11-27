# Optional compatibility kit: formal workbench

This directory preserves the former project-specific acceptance checker, its templates and its regression tests. It is **not** embedded or installed by `mulix init` or `mulix update`. The default workflow uses the native AC/TC matrix and quality runner without requiring a sibling product repository, case index or report layout.

Projects whose constitutions already require this exact protocol should migrate **before** upgrading: keep their approved inputs and verification records, copy the kit into a project-owned location outside the generated skills/templates, and update the project instructions to point to it. Use the [existing protocol](references/acceptance-gate.md) only there; run the project gate in addition to the native runner. Do not adjust old approvals or invent missing observations to fit the checker.

A normal update can retire unchanged formerly shipped files when their vendor baselines are trusted. Customized or unknown content is retained for explicit resolution. Do not force an update to bypass migration or a delivery gate.

The compatibility tests are synthetic and are not acceptance evidence for a consuming project.
