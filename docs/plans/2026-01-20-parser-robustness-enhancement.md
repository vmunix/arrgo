# Parser Robustness Enhancement

**Status**: Pending
**Created**: 2026-01-20
**Estimated Effort**: 6-8 hours total

## Overview

Enhance the release parser to improve accuracy from ~75-80% to 90%+ without replicating Sonarr/Radarr complexity. Uses a lightweight tiered approach: improved normalization, fuzzy title matching, and TMDB fallback.

## Background

### Current State
- **Parser**: `pkg/release/parser.go` (~540 lines)
- **Coverage**: 75-80% overall, 90%+ for standard western releases
- **Architecture**: Regex-based, anchor-detection, destructive normalization

### Known Gaps
1. **Title extraction**: Fails when years appear in titles (e.g., "2012")
2. **Audio detection**: Missing DD.5.1, DD5.1 variants
3. **Service detection**: Inconsistent delimiter handling (DSNP vs D.S.N.P)

### Research Summary
Evaluated 4-tier approach from scene_matching_research.md:
- Tier 1: Regex (current) - ~50µs, 90% coverage
- Tier 2: Fuzzy logic (Bleve + Jaro-Winkler) - ~2ms
- Tier 3: Vector embeddings - ~50ms
- Tier 4: SLM cognitive parsing - ~500ms-2s

**Decision**: Implement lightweight 2.5-tier approach (skip Bleve, embeddings, LLMs for v1).

## Implementation Plan

### Task 1: Title Normalization Enhancements

**Files**: `pkg/release/parser.go`, `pkg/release/parser_test.go`

**Steps**:
1. Add `normalizeRomanNumerals(s string) string` function
   - II → 2, III → 3, IV → 4, etc. (up to X)
   - Only convert when preceded by space/delimiter
   - Handle "Part II", "Chapter III" patterns
2. Add conjunction normalization to existing `CleanTitle()`
   - & → and
   - + → plus (optional)
3. Add unit tests for normalization edge cases:
   - "Back to the Future Part II" → "Back to the Future Part 2"
   - "Fast & Furious" → "Fast and Furious"
   - "Rocky III" → "Rocky 3"

**Acceptance Criteria**:
- [ ] Roman numerals I-X converted correctly
- [ ] Conjunction normalization works
- [ ] Edge cases don't break existing parsing
- [ ] Unit tests pass

### Task 2: Audio Pattern Expansion

**Files**: `pkg/release/parser.go`, `pkg/release/golden_test.go`

**Steps**:
1. Expand `audioPatterns` regex to include variants:
   ```
   DD.5.1, DD5.1, DD51 → Dolby Digital 5.1
   DD+5.1, DDP5.1 → Dolby Digital Plus 5.1
   TrueHD.Atmos, TrueHD-Atmos → TrueHD Atmos
   FLAC.2.0, FLAC2.0 → FLAC 2.0
   ```
2. Add golden test cases for each variant
3. Verify existing tests still pass

**Acceptance Criteria**:
- [ ] All audio variants detected correctly
- [ ] Golden tests include real-world examples
- [ ] No regression in existing tests

### Task 3: Service Detection Hardening

**Files**: `pkg/release/parser.go`, `pkg/release/golden_test.go`

**Steps**:
1. Expand service patterns to handle delimiter variants:
   ```
   DSNP, D.S.N.P, DSNY → Disney+
   AMZN, A.M.Z.N → Amazon
   ATVP, A.T.V.P → Apple TV+
   HMAX, H.M.A.X → HBO Max
   PMTP, P.M.T.P → Paramount+
   ```
2. Add normalization for hyphenated variants
3. Add golden test cases

**Acceptance Criteria**:
- [ ] Service detection handles all common delimiter styles
- [ ] Golden tests cover edge cases
- [ ] No regression

### Task 4: Add go-edlib Dependency

**Files**: `go.mod`, `go.sum`

**Steps**:
1. Run `go get github.com/hbollon/go-edlib`
2. Verify no conflicts with existing dependencies
3. Check license compatibility (MIT)

**Acceptance Criteria**:
- [ ] Dependency added cleanly
- [ ] `go mod tidy` succeeds
- [ ] Build succeeds

### Task 5: Create Fuzzy Title Matcher

**Files**: NEW `pkg/release/matcher.go`, NEW `pkg/release/matcher_test.go`

**Steps**:
1. Create `matcher.go` with:
   ```go
   package release

   import "github.com/hbollon/go-edlib"

   // MatchResult represents a fuzzy match result
   type MatchResult struct {
       Title      string
       Score      float64
       Confidence MatchConfidence
   }

   type MatchConfidence int

   const (
       ConfidenceHigh   MatchConfidence = iota // Score >= 0.95
       ConfidenceMedium                        // Score >= 0.85
       ConfidenceLow                           // Score >= 0.70
       ConfidenceNone                          // Score < 0.70
   )

   // MatchTitle finds the best match for a parsed title against candidates
   func MatchTitle(parsed string, candidates []string) MatchResult {
       // Normalize parsed title
       normalized := CleanTitle(parsed)

       best := MatchResult{Score: 0, Confidence: ConfidenceNone}
       for _, c := range candidates {
           candidateNorm := CleanTitle(c)
           score := edlib.JaroWinklerSimilarity(normalized, candidateNorm)
           if score > best.Score {
               best.Title = c
               best.Score = score
           }
       }

       // Set confidence level
       switch {
       case best.Score >= 0.95:
           best.Confidence = ConfidenceHigh
       case best.Score >= 0.85:
           best.Confidence = ConfidenceMedium
       case best.Score >= 0.70:
           best.Confidence = ConfidenceLow
       }

       return best
   }
   ```
2. Add unit tests for:
   - Exact match (score = 1.0)
   - Close match ("Back to the Future 2" vs "Back to the Future Part II")
   - No match scenario
   - Empty candidates list

**Acceptance Criteria**:
- [ ] MatchTitle returns correct scores
- [ ] Confidence levels set appropriately
- [ ] Unit tests cover edge cases

### Task 6: Add Confidence to ParsedRelease

**Files**: `pkg/release/parser.go`, `pkg/release/types.go` (if exists)

**Steps**:
1. Add `MatchConfidence` field to ParsedRelease struct:
   ```go
   type ParsedRelease struct {
       // ... existing fields ...
       MatchConfidence MatchConfidence `json:"match_confidence,omitempty"`
   }
   ```
2. Update JSON serialization if needed

**Acceptance Criteria**:
- [ ] ParsedRelease includes confidence field
- [ ] JSON serialization works correctly
- [ ] Existing tests pass

### Task 7: Integrate Matcher with Search Flow

**Files**: `internal/search/searcher.go` (or equivalent)

**Steps**:
1. When processing search results:
   - Parse release name
   - Match parsed title against library titles using MatchTitle
   - Set MatchConfidence on result
2. If confidence < ConfidenceMedium, flag for TMDB lookup (Task 8)

**Acceptance Criteria**:
- [ ] Search results include confidence scores
- [ ] Low-confidence matches flagged correctly

### Task 8: TMDB Fallback for Uncertain Matches

**Files**: `internal/search/searcher.go`, existing TMDB client

**Steps**:
1. When MatchConfidence == ConfidenceLow or ConfidenceNone:
   - Query TMDB search API with extracted title + year
   - If single high-confidence result, use it
   - If multiple results, keep as uncertain
2. Add caching for TMDB lookups (24-hour TTL)
3. Add rate limiting if not already present

**Acceptance Criteria**:
- [ ] TMDB fallback triggers for low-confidence matches
- [ ] Results cached to avoid repeated queries
- [ ] Existing TMDB functionality not broken

### Task 9: Integration Tests

**Files**: NEW `pkg/release/integration_test.go` or similar

**Steps**:
1. Create integration test with real-world release names:
   - Standard western movie/TV releases
   - Releases with roman numerals
   - Releases with & conjunctions
   - Various audio format variants
   - Various service patterns
2. Test full flow: parse → match → TMDB fallback
3. Verify 90%+ accuracy on test corpus

**Acceptance Criteria**:
- [ ] Integration tests pass
- [ ] Coverage meets 90% target on test corpus

### Task 10: Update CLI Parse Command

**Files**: `cmd/arrgo/parse.go`

**Steps**:
1. Display MatchConfidence when available
2. Show normalized title (after roman numeral/conjunction conversion)
3. Update help text if needed

**Acceptance Criteria**:
- [ ] CLI shows confidence level
- [ ] Output format is clear and useful

### Task 11: Run Full Test Suite

**Steps**:
1. Run `task test` - all tests must pass
2. Run `task lint` - no new warnings
3. Run `task build` - builds successfully

**Acceptance Criteria**:
- [ ] All tests pass
- [ ] No lint warnings
- [ ] Build succeeds

## Files Changed

| File | Change |
|------|--------|
| `pkg/release/parser.go` | Normalization, pattern expansion |
| `pkg/release/parser_test.go` | Unit tests |
| `pkg/release/golden_test.go` | New golden test cases |
| `pkg/release/matcher.go` | NEW - Fuzzy matching |
| `pkg/release/matcher_test.go` | NEW - Matcher tests |
| `internal/search/searcher.go` | Integration with matcher + TMDB fallback |
| `cmd/arrgo/parse.go` | Display confidence |
| `go.mod`, `go.sum` | go-edlib dependency |

## Dependencies

- `github.com/hbollon/go-edlib` - String distance algorithms (MIT license, ~50KB)

## What NOT to Implement

- **Bleve full-text search** - Overkill for title matching
- **Vector embeddings** - Requires CGO/ONNX, complex deployment
- **LLM/SLM integration** - Way overkill for v1
- **Copying Sonarr/Radarr patterns** - GPL contamination risk, complexity

## Verification

1. `task test` - All tests pass
2. `task lint` - No warnings
3. Manual testing with real indexer results
4. Verify TMDB fallback works for ambiguous titles

## Success Metrics

| Metric | Before | After |
|--------|--------|-------|
| Golden test pass rate | 100% | 100% |
| Real corpus accuracy | 75-80% | 90%+ |
| Standard release accuracy | 90% | 98%+ |
| Code complexity | ~540 lines | ~700 lines |
