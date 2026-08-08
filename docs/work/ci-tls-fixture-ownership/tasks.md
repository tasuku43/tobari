# Work Tasks: Make the synthetic TLS fixture owner-consistent

- [x] Capture the Linux failure and identify the UID mismatch.
- [x] Bind the mock consumer to the fixture owner UID/GID.
- [x] Run local runtime and repository gates.
- [x] Keep HOME-relative project mount scaffolds host-owned and fail closed.
- [x] Own the dev-resolver runtime prerequisite without clobbering local tags.
- [ ] Verify the GitHub container job.
- [ ] Remove the temporary packet.
