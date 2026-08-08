# Work Plan: Make the synthetic TLS fixture owner-consistent

1. Run the synthetic TLS upstream with the host fixture owner's explicit
   UID/GID.
2. Retain owner-only fixture modes and document the harness ownership rule.
3. Pre-create HOME-relative project bind targets under the host owner and add
   unit coverage for unsafe substitutions.
4. Run local runtime and repository gates.
5. Make the dev-resolver runtime prerequisite explicit without replacing a
   contributor's existing dev tag.
6. Push and verify the GitHub container gate.
7. Remove this temporary packet after promoting the conclusion.
