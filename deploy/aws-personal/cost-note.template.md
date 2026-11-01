# Personal always-on cost note

Fill this after the first full calendar month on AWS. The measured total is the
price floor if you later clone one instance per person.

## Hypothesis

Monthly AWS cost + ops effort < cost of leaving a PC on, or of missed alerts.

## Instance

| Field | Value |
| --- | --- |
| Provider | Lightsail / EC2 (circle one) |
| Region | |
| Size / plan | |
| OS | |
| Public hostname | |
| Go-live date | |

## Monthly line items (actual invoice)

| Line item | Amount (currency) | Notes |
| --- | --- | --- |
| Compute | | |
| Disk / storage | | |
| Snapshots / AMI | | |
| Data transfer | | |
| Static IP (if any) | | |
| Domain / DNS (if billed here) | | |
| Other | | |
| **Total** | | |

## Ops effort

| Item | Hours or minutes this month |
| --- | --- |
| Initial setup (amortize) | |
| Updates / restarts | |
| Backup verification | |
| Incident / downtime | |

## Outcome check

| Question | Answer |
| --- | --- |
| Did alerts keep firing with the PC offline? | yes / no |
| Any unplanned downtime hours? | |
| Is total cost acceptable for personal use? | yes / no |
| Would you charge others at least this total + margin per instance? | yes / no / n/a |

## Decision for later hosting

- [ ] Stay personal-only
- [ ] Clone instances for a few trusted people (same runbook)
- [ ] Consider Marketplace (buyer-hosted) instead of paying infra yourself
- [ ] Do not pursue multi-tenant SaaS yet

## Notes

_
