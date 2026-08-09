# Presentation Evidence: Configure narrow Context projections

## Frozen semantic corpus

- Typed fixture path: `internal/cli/testdata/context_configuration_report.json`
- Fixture SHA-256: `a98365dfe0c2d9b87d5793e065e6292f75346bcb67c6201814ec137abc01b533`
- Presentation-independent answer-key path: `internal/cli/testdata/context_configuration_answer.json`
- Answer-key SHA-256: `8fdfb49a3f7498334bc5c9b46ce95cd5d80e06b7e2f8e34c3336b8baea2a0539`
- Declared task and scope: one confirmed `config.git` fixed-target result, its selected Context name/ID, and the complete applied shell/Git policy view
- Interpretation-relevant states: active versus selected Context, default/inherit/literal shell policies, an explicit empty shell literal, and one complete literal Git identity
- Canonical references and exact next argv: no opaque references; successful Git configuration presents exact next argv `tobari`, while mutation-failure recovery remains the catalog-declared read-only `context show`

## Semantic eligibility

- [x] One wizard or direct invocation returns the complete confirmed Context report.
- [x] Routine success requires zero undeclared joins or exploratory calls.
- [x] No opaque canonical reference is introduced or transformed.
- [x] The target Context is a typed input/state fact rather than inferred from labels.
- [x] The corpus preserves explicit empty versus absent values; separate domain and interaction tests cover unresolved inheritance and cancellation.
- [x] Display labels, order, prose, or indentation create no authentication or signing claim.
- [x] Recovery commands obey the exact catalog grammar.

## Reproducible comparison

| Evidence | Before | After |
|---|---:|---:|
| Golden path | Ineligible: the prior shell-only command could not express the atomic Git task | `internal/cli/testdata/context_configuration_report.txt` |
| Golden SHA-256 | Not applicable | `9a30f2ee8a0b6cc2e19a1f59d9626e383f51e33bea201200fb223d72a66bd1d5` |
| UTF-8 bytes | Not applicable | 818 |
| Tokens | Not selected | Not selected |
| Task invocations | 1 | 1 |
| External reconstruction steps | 0 | 0 |

- Golden generator or command: `go test -run TestContextConfigurationPresentationEvidenceUsesOneTypedFixture ./internal/cli`
- Tokenizer name and exact version: Not selected; byte evidence is sufficient for this naming and interaction decision
- Tokenizer configuration: Not applicable
- Platform/runtime facts: raw terminal rendering is Unix-specific; deterministic line-mode golden remains cross-platform
- Invalidation rule: any fixture, answer key, command path, renderer, or wizard-state change invalidates the recorded hashes

## Experiment outcome

- Outcome: combination
- Eligible candidates: configuration-first namespace plus no-setting-flags wizard and complete direct mode
- Failed or invalidated candidates and reasons: context-first was less task-oriented; explicit `set`/`--interactive` added ceremony; missing-field prompting could hang a mistaken script
- Raw evidence retained at: the typed fixture, answer key, text golden, and focused CLI contract test; this packet records the decision until deletion
- Documented gates not implemented by the scorer: catalog/schema/security/runtime checks remain separate deterministic gates

## Product compatibility decision

- Decision owner: Product owner
- Selected presentation: `config shell` and `config git`; no setting flags opens a text terminal wizard, complete flags execute directly
- Compatibility rationale: before v1.0, replace the recently added inconsistent shell path instead of preserving an alias
- Schema/version impact: Context report 5 to 6; Context manifest 4 to 5
- Rollout and rollback: document the rename; schema rollback requires matching older state backup
- Relationship to the experiment outcome: the product owner selected the combined concept after reviewing three namespace and three interaction alternatives

## Security and execution boundary

- [x] Fixtures and evidence use synthetic public-safe values only.
- [x] Final artifact paths are repository-rooted regular files without symbolic links.
- [x] Host Git subprocess contract is purpose-bound, finite, bounded, private, and secret-free.
- [x] No network or live-model evaluation is required.
