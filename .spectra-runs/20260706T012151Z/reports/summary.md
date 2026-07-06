# SPECTRA cat.c to Go Run

- Run id: `20260706T012151Z`
- Provider: `antigravity`
- Model: `gemini-3.5-flash`
- Oracle: `/usr/bin/cat`
- Source: `cat.c`
- Candidates per group: `3`
- Tests: `17`

## Improvement Over Baseline

| k | baseline best@k | spectra best@k | absolute improvement | relative improvement | baseline pass@k | spectra pass@k |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 0.823529 | 0.941176 | 0.117647 | 0.142857 | 0 | 0 |
| 2 | 0.823529 | 1.000000 | 0.176471 | 0.214286 | 0 | 1 |
| 3 | 0.941176 | 1.000000 | 0.058824 | 0.062501 | 0 | 1 |

## Candidate Scores

See `scores.tsv` for machine-readable results.

- `baseline_1`: group=baseline modality=baseline build=built passed=14/17 score=0.823529 full_pass=0
- `baseline_2`: group=baseline modality=baseline build=built passed=14/17 score=0.823529 full_pass=0
- `baseline_3`: group=baseline modality=baseline build=built passed=16/17 score=0.941176 full_pass=0
- `spectra_static_1`: group=spectra modality=static build=built passed=16/17 score=0.941176 full_pass=0
- `spectra_io_1`: group=spectra modality=io build=built passed=17/17 score=1.000000 full_pass=1
- `spectra_descriptions_1`: group=spectra modality=descriptions build=build_failed passed=0/17 score=0.000000 full_pass=0

## Scoring Definition

- `score = passed_tests / total_tests`
- `best@k = max(score)` among candidates in that group with order <= k
- `absolute improvement = spectra best@k - baseline best@k`
- `relative improvement = (spectra best@k - baseline best@k) / baseline best@k`
- `pass@k = 1` if any candidate in that group with order <= k passes every test
