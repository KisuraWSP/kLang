# kLang Compatibility and Removal Policy

Language and standard-library removals follow this sequence:

1. Announce the proposed change in release notes and the language roadmap.
2. Add a checker warning with a stable code, reason, and concrete replacement.
3. Preserve the old behavior through an explicit compatibility implementation.
4. Provide formatter, updater, or documented mechanical migration guidance.
5. Retain the deprecated behavior through at least one language-version
   boundary.
6. Remove it only in a declared breaking language version.
7. Update the backend feature matrix, language specifications, conformance
   fixtures, and migration tests in the same change.

Deprecation does not authorize semantic drift between backends. During the
compatibility window, Standalone, JS, WASM fallback, and bytecode behavior must
remain as recorded in the feature matrix. Unsupported behavior must be rejected
with a source-located diagnostic carrying a stable feature identifier.

An implemented feature is never removed solely because repository search finds
few users. Removal requires an evidence-based audit covering safety, maintenance
cost, replacement completeness, ecosystem impact, and migration feasibility.

Emergency removal before a version boundary is reserved for security or data
loss defects. It requires a published rationale, a safe failure diagnostic, and
the least disruptive migration path available.
