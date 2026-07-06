# SPECTRA `cat.c` to Go Experiment

This repository experiments with translating GNU Coreutils `cat.c` to Go using the SPECTRA method from `spectra.pdf`.

The workflow can use OpenRouter or the local Codex, OpenCode, and Antigravity CLIs to generate baseline and SPECTRA-guided Go packages, then evaluates every generated binary against `/usr/bin/cat`.

## Workflow

SPECTRA compares normal LLM translation against specification-guided translation.

```mermaid
flowchart LR
    A[Source program: cat.c] --> B[Generate specifications]
    B --> C1[Static specs]
    B --> C2[Dynamic I/O specs]
    B --> C3[Natural-language descriptions]
    C1 --> D[Translate with one spec modality]
    C2 --> D
    C3 --> D
    A --> E[Baseline translation: source only]
    D --> F[Generated Go packages]
    E --> F
    F --> G[Build each package]
    G --> H[Run test matrix]
    H --> I[Compare stdout, stderr, and exit status with /usr/bin/cat]
    I --> J[Report best@k and pass@k]
```

The baseline is generated with only the source file:

```text
LLM(cat.c) -> Go candidate
```

The SPECTRA candidates are generated with the source plus exactly one specification modality:

```text
LLM(cat.c + static spec) -> Go candidate
LLM(cat.c + I/O spec) -> Go candidate
LLM(cat.c + description spec) -> Go candidate
```

The script intentionally does not combine all specs in one prompt. The paper reports that larger combined prompts can degrade translation quality.

## Generator strategies

Every backend implements the same provider-neutral generation contract. Specs are
plain Markdown. Candidate packages are returned as strict JSON and validated before
the runner writes `go.mod` and `.go` files. The CLIs run without workspace write
access; OpenRouter uses the official `openai` Python package and requires model
support for structured outputs.

## Run Layout

Each candidate gets an isolated package directory.

```mermaid
flowchart TB
    R[.spectra-runs/run-id]
    R --> S[specs]
    R --> P[packages]
    R --> O[oracle outputs]
    R --> REP[reports]
    P --> B1[baseline_1]
    P --> B2[baseline_2]
    P --> B3[baseline_3]
    P --> ST[spectra_static_1]
    P --> IO[spectra_io_1]
    P --> DS[spectra_descriptions_1]
    REP --> Scores[scores.tsv]
    REP --> Tests[test-results.tsv]
    REP --> Summary[summary.md]
```

## Running

Install the locked environment:

```bash
uv sync --dev
```

Select exactly one provider and model per generation run:

```bash
uv run ./run_spectra_cat_go.py --provider codex --model MODEL
uv run ./run_spectra_cat_go.py --provider opencode --model PROVIDER/MODEL
uv run ./run_spectra_cat_go.py --provider antigravity --model "MODEL LABEL"
OPENROUTER_API_KEY=... uv run ./run_spectra_cat_go.py --provider openrouter --model PROVIDER/MODEL
```

Useful options:

```bash
uv run ./run_spectra_cat_go.py --provider codex --model MODEL --candidates 3
uv run ./run_spectra_cat_go.py --provider codex --model MODEL --generation-timeout 300 --max-retries 2
uv run ./run_spectra_cat_go.py --evaluate-existing .spectra-runs/20260701T015652Z
```

Generated run artifacts are ignored by git under `.spectra-runs/`.
Each new run includes `run.json` with provider/model data, hashes, versions, timings,
and generation-attempt outcomes. Secrets are never written to this manifest.

Executable overrides are available through `CODEX_BIN`, `OPENCODE_BIN`, and
`ANTIGRAVITY_BIN` (which defaults to `agy`).
OpenRouter credentials are read only from `OPENROUTER_API_KEY`.

## Evaluation

The evaluator builds each generated Go package and runs a 17-case test matrix against `/usr/bin/cat`.

Each test compares:

```text
stdout bytes
normalized stderr
exit status
```

The score is:

```text
score = passed_tests / total_tests
best@k = max(score) among the first k candidates in that group
absolute improvement = spectra best@k - baseline best@k
relative improvement = (spectra best@k - baseline best@k) / baseline best@k
```

## Local Results

Valid run:

```text
.spectra-runs/20260701T015652Z
model: openai/gpt-5.4-mini-fast
oracle: /usr/bin/cat
tests: 17
```

Article-style comparison, using number of passing tests out of 17 as the correctness count:

| Method | pass@1 correct | pass@2 correct | pass@3 correct | pass@1 improvement | pass@2 improvement | pass@3 improvement |
|---|---:|---:|---:|---:|---:|---:|
| Baseline | 15 | 16 | 16 | - | - | - |
| SPECTRA | 15 | 17 | 17 | 0% | 6.25% | 6.25% |

Candidate-level results:

| Candidate | Group | Spec modality | Build | Passed tests | Score | Full pass |
|---|---|---|---|---:|---:|---:|
| `baseline_1` | Baseline | none | built | 15/17 | 0.882353 | no |
| `baseline_2` | Baseline | none | built | 16/17 | 0.941176 | no |
| `baseline_3` | Baseline | none | built | 16/17 | 0.941176 | no |
| `spectra_static_1` | SPECTRA | static | built | 15/17 | 0.882353 | no |
| `spectra_io_1` | SPECTRA | I/O | built | 17/17 | 1.000000 | yes |
| `spectra_descriptions_1` | SPECTRA | descriptions | built | 16/17 | 0.941176 | no |

The winning local candidate was `spectra_io_1`. It passed all 17 tests, while the best baseline candidate passed 16 of 17.

## Notes

This experiment is smaller than the paper's benchmark. The paper reports pass@k across hundreds of source programs or functions. This repo currently measures one source program with multiple generated candidates and a focused `/usr/bin/cat` compatibility test matrix.
