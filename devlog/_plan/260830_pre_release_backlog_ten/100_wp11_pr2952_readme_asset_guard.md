# 100 — WP11: PR #2952 README package-asset check

Owner score 42. Author `luvs01`. 1 file, +16/−7.

## Outcome: NOOP — already landed

This was on the shortlist as the cheapest release-artifact hygiene fix. It merged
during the analysis pass and is on `dev` as **`dca16949b`**:

```
dca16949b test: let the README asset check tell files from directories (#2952)
```

`gh pr view 2952` returns `MERGED`.

## What it fixed

The npm README asset guard treated a directory as a satisfied asset, so a
published package could pass the check while shipping a directory where a file was
expected. It now distinguishes the two.

## Why it stays in the plan as its own phase

Recording it as a verified `NOOP` is the honest close. Silently dropping it from
an eleven-item map would make the map disagree with the shortlist the owner
approved, and a future reader could not tell whether it was done or forgotten.

## Done

Confirmed merged as `dca16949b` on `dev`; recorded `NOOP` (criterion c-11).
