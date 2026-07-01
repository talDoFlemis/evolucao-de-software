# SPECTRA cat.c to Go Run

- Run id: `20260701T011204Z`
- Model: `openai/gpt-5.4-mini-fast`
- Oracle: `/usr/bin/cat`
- Source: `cat.c`
- Candidates per group: `1`
- Tests: `17`

## Improvement Over Baseline

| k | baseline best@k | spectra best@k | absolute improvement | relative improvement | baseline pass@k | spectra pass@k |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 0.000000 | 0.000000 | 0.000000 | 0.000000 | 0 | 0 |

## Candidate Scores

See `scores.tsv` for machine-readable results.

- `baseline_1`: group=baseline modality=baseline build=build_failed passed=0/17 score=0.000000 full_pass=0
- `spectra_static_1`: group=spectra modality=static build=build_failed passed=0/17 score=0.000000 full_pass=0

## Scoring Definition

- `score = passed_tests / total_tests`
- `best@k = max(score)` among candidates in that group with order <= k
- `absolute improvement = spectra best@k - baseline best@k`
- `relative improvement = (spectra best@k - baseline best@k) / baseline best@k`
- `pass@k = 1` if any candidate in that group with order <= k passes every test
