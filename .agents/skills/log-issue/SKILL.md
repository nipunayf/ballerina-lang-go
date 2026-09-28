---
name: log-issue
description: File the session's most recent bug/improvement finding under reports/bugs/ or reports/improvements/. Use for "log this as a bug", "file an improvement", "log-issue".
---

Identify, classify, and file the finding as a report under `reports/`.

## 1. Classify and fill the template

Classify without asking: broken/crashing/wrong output → **Bug**; works but
could be better, or missing/awkward → **Improvement**. Fill the matching
`templates/bug.md` or `templates/improvement.md` from
the finding. Write valid Markdown; keep it brief and direct, with only the
information needed to act on it.

## 2. Write the report

- Slugify the title: lowercase, hyphens, no punctuation.
- Write to `reports/bugs/<slug>.md` or `reports/improvements/<slug>.md` at the
  repo root, creating either directory if it doesn't exist yet.
  
