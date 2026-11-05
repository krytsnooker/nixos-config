# Unstable overrides registry

Any package pulled from `nixpkgs-unstable` instead of the primary `nixpkgs` input gets exactly one row here. The table and the procedure below are app-agnostic — the same four columns and the same three checks apply regardless of which package or why it's there.

## Registry

| attr | input | reason | floor |
|---|---|---|---|
| `nextcloud34` | `nixpkgs-unstable` | Android app / desktop client required a newer server version than the stable branch carried at pin time | server ≥ whatever version the currently-required client build needs |
| `vaultwarden.webvault` | `nixpkgs-unstable` | browser extension required a newer webvault build than the stable branch carried at pin time | webvault ≥ whatever version the currently-required extension build needs |
| `brave-origin` | `nixpkgs-unstable` | just wants newer Brave releases, no hard compatibility requirement | REMOVED 2026-11-03 (nixpkgs: no committers to unblock security issues) — watch for reinstatement |

`floor` is intentionally vague above — fill in the actual version number the first time you verify it (see Step 1 below). A row with no real constraint (like `brave-origin`) just carries `none` permanently; it never generates a migration trigger, it's only ever re-checked for raw staleness on whatever cadence you run `nixpkgs-unstable` bumps on.

## Procedure — identical for every row, run on the same cadence as your `nixpkgs-unstable` bump

**Step 1 — Check the floor is still satisfied.**
If `floor` is set, confirm the currently locked version still clears it. If it has no floor, skip this — just rely on the normal age-based bump cadence for the input overall.

**Step 2 — Check whether the primary input now clears the floor.**
Compare `floor` against what's available on the `nixpkgs` (stable) branch tip:
```
nix eval --raw 'github:NixOS/nixpkgs/<stable-branch-or-rev>#<attr>.version'
```
If the stable branch now carries a version ≥ `floor`, this row is done: in the consuming module, switch the package reference from `pkgsUnstable.<attr>` back to plain `pkgs.<attr>`, update the attribute name if the stable branch exposes it under a different name (e.g. a new major-version suffix), and delete the row.

**Step 3 — No floor, just chasing freshness.**
Rows with `floor: none` never trigger migration — they stay on unstable by choice. Just make sure they're not silently drifting for months; they ride the same `nixpkgs-unstable` bump cadence as everything else in this table.

**Step 4 — Package removed upstream entirely (not just behind).**
If `check-updates.sh` reports `REMOVED/ERR` for a row's `channel-tip`/`raw-tip` — the attribute now throws on evaluation — this isn't a staleness problem a lock bump fixes; nixpkgs dropped the package outright (usually: no maintainer left to handle security issues). Set `floor` to `REMOVED <date> (<reason>) — watch for reinstatement` and take over the package with a local custom derivation, referenced directly (not via `pkgsUnstable.<attr>`) from its consuming module.
Once a row reaches this state, its attr is no longer referenced anywhere in code — the module-scan in `check-updates.sh` that discovers `UNSTABLE_ATTRS` can't find it anymore. **This table's `attr` column is itself a second source `check-updates.sh` reads**, specifically so rows like this keep getting checked after you've taken over maintaining them — if nixpkgs ever re-adds the package (someone adopts it) and this row's attr starts resolving to a real version again, that's your signal the burden can be handed back: migrate back to `pkgs.<attr>` or `pkgsUnstable.<attr>` as appropriate and delete the row.

## Why this exists

Pinning a package to `nixpkgs-unstable` is cheap to do and easy to forget — nothing marks it as a deliberate, time-bound decision once it's merged. This file is the thing that remembers *why* a package is there and *what would let it leave*, independent of whoever's memory would otherwise have to reconstruct it later.
