## Summary

<!-- Briefly describe what this PR does. -->

## Checklist

**If this PR adds a new trading rule:**

- [ ] `internal/methodology/<category>/<rule_name>.go` created
- [ ] `var _ rule.AnalysisRule = (*MyRule)(nil)` compile-time assertion included
- [ ] Rule registered in `config/rules.yaml`
- [ ] `_test.go` with table-driven tests (at minimum: signal fires, signal does not fire, too few bars)

**All PRs:**

- [ ] `go test ./...` passes locally
- [ ] `go vet ./...` produces no warnings
- [ ] `CHANGELOG.md` updated
- [ ] No `.env` file or real API keys in the diff

<!-- Items that don't apply to your PR can be marked N/A. -->
