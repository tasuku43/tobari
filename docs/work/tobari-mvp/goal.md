# Work Goal: Deliver the Tobari MVP

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: docs/00_theses.md
- Review/delete trigger: Delete after durable conclusions are promoted and the change completes
- Successor: None
- Owner: Tobari maintainers
- Target: MVP
- Related ADRs: To be added in this packet

## Outcome

A developer can build `tobari`, start one Docker-isolated Realm for a selected
root, run concurrent tools inside it, and rely on Gateway plus OPA to deny
direct egress, authorize proxied HTTP/HTTPS, and inject only host-bound managed
credentials.

## Why now

The repository is greenfield and the initiating specification defines a
complete executable MVP rather than a design exercise.

## Non-goals

- Multiple realms or repository/process isolation.
- Transparent proxying or non-HTTP protocols.
- Provider-specific API semantics, OAuth, approval flows, overlays, or GUI.

## Acceptance criteria

- [ ] All public commands build, are discoverable, and behave as documented.
- [ ] HTTP/HTTPS allow and deny work through actual Docker containers.
- [ ] Direct egress and Realm access to control/credential surfaces fail.
- [ ] Gateway and OPA fail closed, and managed credentials never enter Realm or logs.
- [ ] Unit, policy, Gateway, integration, full, security, and public gates pass.

## Governing documents

- Thesis: [Project Theses](../../00_theses.md)
- Product contract: [Product Contract](../../01_product_contract.md)
- Architecture: [Architecture](../../02_architecture.md)
- Security: [Security Model](../../03_security_model.md)

## Completion definition

The work is complete only when every acceptance criterion has evidence,
durable decisions are promoted, all required gates pass, the Quick Start is
replayed, and this temporary packet is removed.
