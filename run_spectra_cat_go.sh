#!/usr/bin/env bash

set -Eeuo pipefail
IFS=$'\n\t'

SCRIPT_NAME="$(basename "$0")"

MODEL="${OPENCODE_MODEL:-}"
CANDIDATES=3
RUN_ROOT=".spectra-runs"
RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
SOURCE_FILE="cat.c"
ORACLE="/usr/bin/cat"
OPENCODE_BIN="${OPENCODE_BIN:-opencode}"
AUTO_APPROVE=0
TEST_TIMEOUT=5
OPENCODE_TIMEOUT="${OPENCODE_TIMEOUT:-600}"
EVALUATE_EXISTING=""

usage() {
  cat <<EOF
Usage: $SCRIPT_NAME [options]

Run a SPECTRA-style C-to-Go translation workflow for cat.c using opencode.
Each opencode candidate writes a separate Go package, then the script scores
the generated binaries against /usr/bin/cat.

Options:
  --model provider/model        opencode model to use. If omitted, prompts.
  --candidates N               Number of baseline candidates and SPECTRA
                               candidates to generate. Default: $CANDIDATES
  --run-id ID                  Run id. Default: UTC timestamp.
  --run-root DIR               Directory for run outputs. Default: $RUN_ROOT
  --source FILE                Source C file. Default: $SOURCE_FILE
  --oracle FILE                Oracle cat executable. Default: $ORACLE
  --opencode FILE              opencode executable. Default: $OPENCODE_BIN
  --timeout SECONDS            Per-test timeout. Default: $TEST_TIMEOUT
  --opencode-timeout SECONDS   Per-opencode-call timeout. Default: $OPENCODE_TIMEOUT
  --evaluate-existing DIR      Rebuild and rescore an existing run directory without
                               running opencode generation again.
  --auto-approve               Pass --dangerously-skip-permissions to opencode.
                               Intended for unattended runs in isolated dirs.
  -h, --help                   Show this help.

Examples:
  $SCRIPT_NAME --model openai/gpt-5.5 --candidates 3
  $SCRIPT_NAME --model anthropic/claude-sonnet-4-5 --auto-approve

Outputs:
  $RUN_ROOT/<run-id>/packages/  Generated Go packages
  $RUN_ROOT/<run-id>/reports/   scores.tsv, test-results.tsv, summary.md
EOF
}

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

warn() {
  printf 'warning: %s\n' "$*" >&2
}

info() {
  printf '%s\n' "$*" >&2
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

abs_path() {
  local path="$1"
  local dir
  local base

  dir="$(dirname "$path")"
  base="$(basename "$path")"
  [[ -e "$path" ]] || die "path does not exist: $path"
  printf '%s/%s' "$(cd "$dir" && pwd -P)" "$base"
}

parse_positive_int() {
  local name="$1"
  local value="$2"
  [[ "$value" =~ ^[1-9][0-9]*$ ]] || die "$name must be a positive integer, got: $value"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --model)
      [[ $# -ge 2 ]] || die "--model requires a value"
      MODEL="$2"
      shift 2
      ;;
    --candidates)
      [[ $# -ge 2 ]] || die "--candidates requires a value"
      parse_positive_int "--candidates" "$2"
      CANDIDATES="$2"
      shift 2
      ;;
    --run-id)
      [[ $# -ge 2 ]] || die "--run-id requires a value"
      RUN_ID="$2"
      shift 2
      ;;
    --run-root)
      [[ $# -ge 2 ]] || die "--run-root requires a value"
      RUN_ROOT="$2"
      shift 2
      ;;
    --source)
      [[ $# -ge 2 ]] || die "--source requires a value"
      SOURCE_FILE="$2"
      shift 2
      ;;
    --oracle)
      [[ $# -ge 2 ]] || die "--oracle requires a value"
      ORACLE="$2"
      shift 2
      ;;
    --opencode)
      [[ $# -ge 2 ]] || die "--opencode requires a value"
      OPENCODE_BIN="$2"
      shift 2
      ;;
    --timeout)
      [[ $# -ge 2 ]] || die "--timeout requires a value"
      parse_positive_int "--timeout" "$2"
      TEST_TIMEOUT="$2"
      shift 2
      ;;
    --opencode-timeout)
      [[ $# -ge 2 ]] || die "--opencode-timeout requires a value"
      parse_positive_int "--opencode-timeout" "$2"
      OPENCODE_TIMEOUT="$2"
      shift 2
      ;;
    --evaluate-existing)
      [[ $# -ge 2 ]] || die "--evaluate-existing requires a value"
      EVALUATE_EXISTING="$2"
      shift 2
      ;;
    --auto-approve)
      AUTO_APPROVE=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown argument: $1"
      ;;
  esac
done

[[ -f "$SOURCE_FILE" ]] || die "source file not found: $SOURCE_FILE"
[[ -x "$ORACLE" ]] || die "oracle is not executable: $ORACLE"
require_command "$OPENCODE_BIN"
require_command go
require_command base64
require_command awk
require_command cmp

if [[ -z "$MODEL" ]]; then
  if [[ -t 0 ]]; then
    printf 'opencode model (provider/model): ' >&2
    read -r MODEL
  else
    die "--model is required when stdin is not interactive"
  fi
fi

[[ -n "$MODEL" ]] || die "model cannot be empty"

if [[ -n "$EVALUATE_EXISTING" ]]; then
  [[ -d "$EVALUATE_EXISTING" ]] || die "existing run directory not found: $EVALUATE_EXISTING"
  RUN_DIR="$EVALUATE_EXISTING"
  RUN_ID="$(basename "$RUN_DIR")"
else
  RUN_DIR="$RUN_ROOT/$RUN_ID"
fi
SPECS_DIR="$RUN_DIR/specs"
PROMPTS_DIR="$RUN_DIR/prompts"
LOGS_DIR="$RUN_DIR/logs"
PACKAGES_DIR="$RUN_DIR/packages"
REPORTS_DIR="$RUN_DIR/reports"
BIN_DIR="$RUN_DIR/bin"
TESTS_DIR="$RUN_DIR/tests"
ORACLE_DIR="$RUN_DIR/oracle"

mkdir -p "$SPECS_DIR" "$PROMPTS_DIR" "$LOGS_DIR" "$PACKAGES_DIR" "$REPORTS_DIR" "$BIN_DIR" "$TESTS_DIR" "$ORACLE_DIR"
if [[ -z "$EVALUATE_EXISTING" ]]; then
  cp "$SOURCE_FILE" "$RUN_DIR/cat.c"
fi

TEST_NAMES=(
  no_args_stdin
  plain_file
  multiple_files
  dash_mixed
  number_all
  number_nonblank
  number_nonblank_overrides_number
  squeeze_blank
  show_ends
  show_tabs
  show_nonprinting
  show_all
  option_e
  option_t
  ignored_u
  long_options
  missing_file
)

prepare_tests() {
  local dir
  printf '' > "$TESTS_DIR/empty.stdin"

  dir="$TESTS_DIR/no_args_stdin"
  mkdir -p "$dir"
  printf 'stdin line 1\nstdin line 2\n' > "$dir/stdin.txt"

  dir="$TESTS_DIR/plain_file"
  mkdir -p "$dir"
  printf 'hello\nworld\n' > "$dir/a.txt"

  dir="$TESTS_DIR/multiple_files"
  mkdir -p "$dir"
  printf 'first\n' > "$dir/a.txt"
  printf 'second\nthird\n' > "$dir/b.txt"

  dir="$TESTS_DIR/dash_mixed"
  mkdir -p "$dir"
  printf 'file-a\n' > "$dir/a.txt"
  printf 'stdin-middle\n' > "$dir/stdin.txt"
  printf 'file-b\n' > "$dir/b.txt"

  dir="$TESTS_DIR/number_all"
  mkdir -p "$dir"
  printf 'alpha\n\nbeta\n' > "$dir/a.txt"

  dir="$TESTS_DIR/number_nonblank"
  mkdir -p "$dir"
  printf 'alpha\n\nbeta\n' > "$dir/a.txt"

  dir="$TESTS_DIR/number_nonblank_overrides_number"
  mkdir -p "$dir"
  printf 'alpha\n\nbeta\n' > "$dir/a.txt"

  dir="$TESTS_DIR/squeeze_blank"
  mkdir -p "$dir"
  printf 'a\n\n\n\nb\n\n\n' > "$dir/a.txt"

  dir="$TESTS_DIR/show_ends"
  mkdir -p "$dir"
  printf 'a\n\ncrlf\r\nno-newline' > "$dir/a.txt"

  dir="$TESTS_DIR/show_tabs"
  mkdir -p "$dir"
  printf 'a\tb\n\tindent\n' > "$dir/a.txt"

  dir="$TESTS_DIR/show_nonprinting"
  mkdir -p "$dir"
  printf '\001\002\037\177\200\377\nplain\ttext\n' > "$dir/a.bin"

  dir="$TESTS_DIR/show_all"
  mkdir -p "$dir"
  printf 'a\tb\n\001\177\n' > "$dir/a.bin"

  dir="$TESTS_DIR/option_e"
  mkdir -p "$dir"
  printf 'x\n\001\n' > "$dir/a.bin"

  dir="$TESTS_DIR/option_t"
  mkdir -p "$dir"
  printf 'x\ty\n\001\t\n' > "$dir/a.bin"

  dir="$TESTS_DIR/ignored_u"
  mkdir -p "$dir"
  printf 'buffering flag is ignored\n' > "$dir/a.txt"

  dir="$TESTS_DIR/long_options"
  mkdir -p "$dir"
  printf 'alpha\n\tbeta\n' > "$dir/a.txt"

  dir="$TESTS_DIR/missing_file"
  mkdir -p "$dir"
  printf 'before\n' > "$dir/a.txt"
}

TEST_STDIN=""
TEST_ARGS=()

configure_test() {
  local test_name="$1"
  local dir="$TESTS_DIR/$test_name"
  TEST_STDIN="$TESTS_DIR/empty.stdin"
  TEST_ARGS=()

  case "$test_name" in
    no_args_stdin)
      TEST_STDIN="$dir/stdin.txt"
      ;;
    plain_file)
      TEST_ARGS=("$dir/a.txt")
      ;;
    multiple_files)
      TEST_ARGS=("$dir/a.txt" "$dir/b.txt")
      ;;
    dash_mixed)
      TEST_STDIN="$dir/stdin.txt"
      TEST_ARGS=("$dir/a.txt" "-" "$dir/b.txt")
      ;;
    number_all)
      TEST_ARGS=("-n" "$dir/a.txt")
      ;;
    number_nonblank)
      TEST_ARGS=("-b" "$dir/a.txt")
      ;;
    number_nonblank_overrides_number)
      TEST_ARGS=("-b" "-n" "$dir/a.txt")
      ;;
    squeeze_blank)
      TEST_ARGS=("-s" "$dir/a.txt")
      ;;
    show_ends)
      TEST_ARGS=("-E" "$dir/a.txt")
      ;;
    show_tabs)
      TEST_ARGS=("-T" "$dir/a.txt")
      ;;
    show_nonprinting)
      TEST_ARGS=("-v" "$dir/a.bin")
      ;;
    show_all)
      TEST_ARGS=("-A" "$dir/a.bin")
      ;;
    option_e)
      TEST_ARGS=("-e" "$dir/a.bin")
      ;;
    option_t)
      TEST_ARGS=("-t" "$dir/a.bin")
      ;;
    ignored_u)
      TEST_ARGS=("-u" "$dir/a.txt")
      ;;
    long_options)
      TEST_ARGS=("--number" "--show-tabs" "$dir/a.txt")
      ;;
    missing_file)
      TEST_ARGS=("$dir/a.txt" "$dir/does-not-exist.txt")
      ;;
    *)
      die "unknown test: $test_name"
      ;;
  esac
}

normalize_stderr() {
  local input="$1"
  local output="$2"
  local line
  : > "$output"
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" == *": "* ]]; then
      printf '<cmd>:%s\n' "${line#*:}" >> "$output"
    else
      printf '%s\n' "$line" >> "$output"
    fi
  done < "$input"
}

execute_case() {
  local binary="$1"
  local test_name="$2"
  local out_dir="$3"
  local status

  mkdir -p "$out_dir"
  configure_test "$test_name"

  set +e
  if command -v timeout >/dev/null 2>&1; then
    timeout "$TEST_TIMEOUT" "$binary" "${TEST_ARGS[@]}" \
      < "$TEST_STDIN" \
      > "$out_dir/stdout" \
      2> "$out_dir/stderr"
    status=$?
  else
    "$binary" "${TEST_ARGS[@]}" \
      < "$TEST_STDIN" \
      > "$out_dir/stdout" \
      2> "$out_dir/stderr"
    status=$?
  fi
  set -e

  printf '%s\n' "$status" > "$out_dir/status"
  normalize_stderr "$out_dir/stderr" "$out_dir/stderr.norm"
}

generate_oracle_outputs() {
  local test_name
  for test_name in "${TEST_NAMES[@]}"; do
    execute_case "$ORACLE" "$test_name" "$ORACLE_DIR/$test_name"
  done
}

args_for_markdown() {
  local test_name="$1"
  local out=""
  local arg
  configure_test "$test_name"
  for arg in "${TEST_ARGS[@]}"; do
    out+="$(printf '%q' "$arg") "
  done
  printf '%s' "${out% }"
}

base64_file() {
  base64 -w 0 "$1"
}

write_io_spec() {
  local out="$SPECS_DIR/io.md"
  local test_name
  local status

  {
    printf '# Dynamic I/O Specifications\n\n'
    printf 'These examples were generated by executing the oracle `%s`.\n' "$ORACLE"
    printf 'A Go translation should match stdout bytes, normalized stderr, and exit status.\n\n'

    for test_name in "${TEST_NAMES[@]}"; do
      status="$(<"$ORACLE_DIR/$test_name/status")"
      printf '## %s\n\n' "$test_name"
      printf -- '- Args: `%s`\n' "$(args_for_markdown "$test_name")"
      printf -- '- Stdin file: `%s`\n' "$TEST_STDIN"
      printf -- '- Expected exit status: `%s`\n' "$status"
      printf -- '- Expected stdout base64: `%s`\n' "$(base64_file "$ORACLE_DIR/$test_name/stdout")"
      printf -- '- Expected normalized stderr base64: `%s`\n\n' "$(base64_file "$ORACLE_DIR/$test_name/stderr.norm")"
    done
  } > "$out"
}

run_opencode_capture() {
  local title="$1"
  local work_dir="$2"
  local prompt_file="$3"
  local output_file="$4"
  shift 4
  local prompt
  local cmd
  local attached_file
  local status

  prompt="$(<"$prompt_file")"
  cmd=("$OPENCODE_BIN" run "$prompt" --dir "$work_dir" --title "$title" -m "$MODEL")
  for attached_file in "$@"; do
    cmd+=(--file "$(abs_path "$attached_file")")
  done
  info "starting opencode step: $title (timeout ${OPENCODE_TIMEOUT}s)"
  set +e
  if command -v timeout >/dev/null 2>&1; then
    timeout --foreground "$OPENCODE_TIMEOUT" "${cmd[@]}" > "$output_file" 2> "$LOGS_DIR/$title.stderr.log"
    status=$?
  else
    "${cmd[@]}" > "$output_file" 2> "$LOGS_DIR/$title.stderr.log"
    status=$?
  fi
  set -e

  if [[ "$status" -ne 0 ]]; then
    warn "opencode step failed: $title; see $LOGS_DIR/$title.stderr.log"
    if [[ "$status" -eq 124 ]]; then
      warn "opencode step timed out after ${OPENCODE_TIMEOUT}s: $title"
    fi
    printf '\n\n<!-- opencode step failed; see logs -->\n' >> "$output_file"
    return "$status"
  fi

  if [[ ! -s "$output_file" ]]; then
    warn "opencode step produced empty output: $title"
    return 1
  fi
}

run_opencode_writer() {
  local title="$1"
  local work_dir="$2"
  local prompt_file="$3"
  shift 3
  local prompt
  local cmd
  local attached_file
  local status

  prompt="$(<"$prompt_file")"
  cmd=("$OPENCODE_BIN" run "$prompt" --dir "$work_dir" --title "$title" -m "$MODEL")
  if [[ "$AUTO_APPROVE" -eq 1 ]]; then
    cmd+=(--dangerously-skip-permissions)
  fi
  for attached_file in "$@"; do
    cmd+=(--file "$(abs_path "$attached_file")")
  done
  info "starting opencode candidate: $title (timeout ${OPENCODE_TIMEOUT}s)"
  set +e
  if command -v timeout >/dev/null 2>&1; then
    timeout --foreground "$OPENCODE_TIMEOUT" "${cmd[@]}" > "$LOGS_DIR/$title.stdout.log" 2> "$LOGS_DIR/$title.stderr.log"
    status=$?
  else
    "${cmd[@]}" > "$LOGS_DIR/$title.stdout.log" 2> "$LOGS_DIR/$title.stderr.log"
    status=$?
  fi
  set -e

  if [[ "$status" -ne 0 ]]; then
    warn "opencode candidate failed: $title; see $LOGS_DIR/$title.stderr.log"
    if [[ "$status" -eq 124 ]]; then
      warn "opencode candidate timed out after ${OPENCODE_TIMEOUT}s: $title"
    fi
    return "$status"
  fi
}

generate_specs() {
  local prompt_file

  prompt_file="$PROMPTS_DIR/source-map.prompt.md"
  cat > "$prompt_file" <<'EOF'
You are the source-map agent for a SPECTRA-style C-to-Go translation.

Read the attached cat.c and output markdown only. Do not modify files.

Create a concise source map with:
- global state and how it affects behavior across files
- each function and its semantic responsibility
- C/coreutils implementation details that can be simplified in Go
- translation risks that must be tested
EOF
  run_opencode_capture "source-map" "$RUN_DIR" "$prompt_file" "$RUN_DIR/00-source-map.md" "$RUN_DIR/cat.c"

  prompt_file="$PROMPTS_DIR/static-spec.prompt.md"
  cat > "$prompt_file" <<'EOF'
You are the static-specification agent for SPECTRA.

Read the attached cat.c and output markdown only. Do not modify files.

Generate static specifications for translating cat.c to Go. Use this structure:
- Program input/output contract
- Option parsing contract
- Preconditions and postconditions for simple_cat behavior
- Preconditions and postconditions for formatted cat behavior
- Invariants for line numbering, squeeze blank, show tabs, show ends, and show nonprinting
- Explicit equivalences for -A, -e, -t, and ignored -u

Keep the specs precise enough to guide translation but shorter than the source code.
EOF
  run_opencode_capture "static-spec" "$RUN_DIR" "$prompt_file" "$SPECS_DIR/static.md" "$RUN_DIR/cat.c"

  prompt_file="$PROMPTS_DIR/descriptions.prompt.md"
  cat > "$prompt_file" <<'EOF'
You are the natural-language description agent for SPECTRA.

Read the attached cat.c and output markdown only. Do not modify files.

Write concise natural-language descriptions for the Go translator:
- What the program does at a user-visible level
- How each supported option changes output
- What state must persist across input files
- Which C-specific optimizations are not required for semantic parity in Go
- Which edge cases are most likely to be mistranslated
EOF
  run_opencode_capture "descriptions" "$RUN_DIR" "$prompt_file" "$SPECS_DIR/descriptions.md" "$RUN_DIR/cat.c"
}

candidate_modality_for_order() {
  local order="$1"
  case $(( (order - 1) % 3 )) in
    0) printf 'static' ;;
    1) printf 'io' ;;
    2) printf 'descriptions' ;;
  esac
}

setup_candidate_context() {
  local candidate="$1"
  local modality="$2"
  local pkg_dir="$PACKAGES_DIR/$candidate"
  mkdir -p "$pkg_dir/context"
  cp "$RUN_DIR/cat.c" "$pkg_dir/context/cat.c"

  if [[ "$modality" != "baseline" ]]; then
    cp "$SPECS_DIR/$modality.md" "$pkg_dir/context/spec.md"
  fi

  printf '%s\n' "$candidate" > "$pkg_dir/context/candidate-name.txt"
  printf '%s\n' "$modality" > "$pkg_dir/context/modality.txt"
}

write_candidate_prompt() {
  local candidate="$1"
  local group="$2"
  local modality="$3"
  local prompt_file="$PACKAGES_DIR/$candidate/TASK.md"

  if [[ "$group" == "baseline" ]]; then
    cat > "$prompt_file" <<EOF
You are an isolated baseline translation agent.

Task: translate context/cat.c into a standalone idiomatic Go command-line program.

Inputs available in this package directory:
- context/cat.c: the only source context you may use

Output requirements:
- Create a new Go package in the current directory.
- Use package main.
- Write go.mod and one or more .go files.
- Do not use cgo.
- Do not use third-party dependencies.
- Do not edit files under context/.
- The command should be compatible with GNU cat for the implemented behavior.
- Implement the options visible in cat.c: -A, -b, -e, -E, -n, -s, -t, -T, -u, -v and their long forms.
- The program must build with: go build .

Important: this is the baseline arm. Do not use generated SPECTRA specifications.
EOF
  else
    cat > "$prompt_file" <<EOF
You are an isolated SPECTRA translation agent.

Task: translate context/cat.c into a standalone idiomatic Go command-line program using exactly one validated specification modality.

Inputs available in this package directory:
- context/cat.c: source code to translate
- context/spec.md: the ${modality} SPECTRA specification for this candidate

Output requirements:
- Create a new Go package in the current directory.
- Use package main.
- Write go.mod and one or more .go files.
- Do not use cgo.
- Do not use third-party dependencies.
- Do not edit files under context/.
- The command should be compatible with GNU cat for the implemented behavior.
- Implement the options visible in cat.c: -A, -b, -e, -E, -n, -s, -t, -T, -u, -v and their long forms.
- Preserve state across multiple input files where cat.c does so.
- The program must build with: go build .

SPECTRA rule: use the attached ${modality} spec to guide translation. Do not combine other spec modalities.
EOF
  fi
}

generate_candidate() {
  local candidate="$1"
  local group="$2"
  local modality="$3"
  local pkg_dir="$PACKAGES_DIR/$candidate"
  local prompt_file="$pkg_dir/TASK.md"
  local -a attachments

  setup_candidate_context "$candidate" "$modality"
  write_candidate_prompt "$candidate" "$group" "$modality"

  info "running opencode candidate: $candidate ($group/$modality)"
  attachments=("$prompt_file" "$pkg_dir/context/cat.c")
  if [[ "$modality" != "baseline" ]]; then
    attachments+=("$pkg_dir/context/spec.md")
  fi
  run_opencode_writer "$candidate" "$pkg_dir" "$prompt_file" "${attachments[@]}"
}

build_candidate() {
  local candidate="$1"
  local pkg_dir="$PACKAGES_DIR/$candidate"
  local binary="$(abs_path "$BIN_DIR")/$candidate"
  local build_log="$REPORTS_DIR/$candidate.build.log"

  if [[ ! -f "$pkg_dir/go.mod" ]]; then
    printf 'missing go.mod\n' > "$build_log"
    return 1
  fi

  set +e
  (
    cd "$pkg_dir"
    go mod tidy
    go build -o "$binary" .
  ) > "$build_log" 2>&1
  local status=$?
  set -e

  [[ "$status" -eq 0 && -x "$binary" ]]
}

score_candidate() {
  local candidate="$1"
  local group="$2"
  local group_order="$3"
  local modality="$4"
  local total="${#TEST_NAMES[@]}"
  local passed=0
  local build_status="built"
  local test_name
  local candidate_out_dir
  local expected_dir
  local status_actual
  local status_expected
  local stdout_match
  local stderr_match
  local status_match
  local test_pass
  local score
  local full_pass

  if ! build_candidate "$candidate"; then
    build_status="build_failed"
    score="0.000000"
    full_pass=0
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
      "$group" "$group_order" "$candidate" "$modality" "$build_status" 0 "$total" "$score" "$full_pass" \
      >> "$REPORTS_DIR/scores.tsv"
    return 0
  fi

  for test_name in "${TEST_NAMES[@]}"; do
    candidate_out_dir="$REPORTS_DIR/candidate-output/$candidate/$test_name"
    expected_dir="$ORACLE_DIR/$test_name"
    execute_case "$(abs_path "$BIN_DIR")/$candidate" "$test_name" "$candidate_out_dir"

    status_actual="$(<"$candidate_out_dir/status")"
    status_expected="$(<"$expected_dir/status")"
    stdout_match=0
    stderr_match=0
    status_match=0
    test_pass=0

    if cmp -s "$candidate_out_dir/stdout" "$expected_dir/stdout"; then
      stdout_match=1
    fi
    if cmp -s "$candidate_out_dir/stderr.norm" "$expected_dir/stderr.norm"; then
      stderr_match=1
    fi
    if [[ "$status_actual" == "$status_expected" ]]; then
      status_match=1
    fi
    if [[ "$stdout_match" -eq 1 && "$stderr_match" -eq 1 && "$status_match" -eq 1 ]]; then
      test_pass=1
      passed=$((passed + 1))
    fi

    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
      "$candidate" "$test_name" "$test_pass" "$stdout_match" "$stderr_match" "$status_match" "$status_expected" "$status_actual" "$candidate_out_dir" \
      >> "$REPORTS_DIR/test-results.tsv"
  done

  score="$(awk -v passed="$passed" -v total="$total" 'BEGIN { printf "%.6f", passed / total }')"
  if [[ "$passed" -eq "$total" ]]; then
    full_pass=1
  else
    full_pass=0
  fi

  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$group" "$group_order" "$candidate" "$modality" "$build_status" "$passed" "$total" "$score" "$full_pass" \
    >> "$REPORTS_DIR/scores.tsv"
}

best_score_at_k() {
  local group="$1"
  local k="$2"
  awk -F '\t' -v group="$group" -v k="$k" '
    NR > 1 && $1 == group && ($2 + 0) <= k {
      score = $8 + 0
      if (score > best) best = score
    }
    END { printf "%.6f", best + 0 }
  ' "$REPORTS_DIR/scores.tsv"
}

any_full_pass_at_k() {
  local group="$1"
  local k="$2"
  awk -F '\t' -v group="$group" -v k="$k" '
    NR > 1 && $1 == group && ($2 + 0) <= k && $9 == "1" { found = 1 }
    END { print found ? 1 : 0 }
  ' "$REPORTS_DIR/scores.tsv"
}

write_summary() {
  local summary="$REPORTS_DIR/summary.md"
  local k
  local baseline_best
  local spectra_best
  local absolute
  local relative
  local baseline_pass
  local spectra_pass

  {
    printf '# SPECTRA cat.c to Go Run\n\n'
    printf -- '- Run id: `%s`\n' "$RUN_ID"
    printf -- '- Model: `%s`\n' "$MODEL"
    printf -- '- Oracle: `%s`\n' "$ORACLE"
    printf -- '- Source: `%s`\n' "$SOURCE_FILE"
    printf -- '- Candidates per group: `%s`\n' "$CANDIDATES"
    printf -- '- Tests: `%s`\n\n' "${#TEST_NAMES[@]}"

    printf '## Improvement Over Baseline\n\n'
    printf '| k | baseline best@k | spectra best@k | absolute improvement | relative improvement | baseline pass@k | spectra pass@k |\n'
    printf '|---:|---:|---:|---:|---:|---:|---:|\n'
    for ((k = 1; k <= CANDIDATES; k++)); do
      baseline_best="$(best_score_at_k baseline "$k")"
      spectra_best="$(best_score_at_k spectra "$k")"
      absolute="$(awk -v s="$spectra_best" -v b="$baseline_best" 'BEGIN { printf "%.6f", s - b }')"
      relative="$(awk -v s="$spectra_best" -v b="$baseline_best" 'BEGIN { if (b == 0) { if (s == 0) printf "0.000000"; else printf "inf" } else { printf "%.6f", (s - b) / b } }')"
      baseline_pass="$(any_full_pass_at_k baseline "$k")"
      spectra_pass="$(any_full_pass_at_k spectra "$k")"
      printf '| %s | %s | %s | %s | %s | %s | %s |\n' \
        "$k" "$baseline_best" "$spectra_best" "$absolute" "$relative" "$baseline_pass" "$spectra_pass"
    done

    printf '\n## Candidate Scores\n\n'
    printf 'See `scores.tsv` for machine-readable results.\n\n'
    awk -F '\t' 'NR == 1 { next } { printf "- `%s`: group=%s modality=%s build=%s passed=%s/%s score=%s full_pass=%s\n", $3, $1, $4, $5, $6, $7, $8, $9 }' "$REPORTS_DIR/scores.tsv"

    printf '\n## Scoring Definition\n\n'
    printf -- '- `score = passed_tests / total_tests`\n'
    printf -- '- `best@k = max(score)` among candidates in that group with order <= k\n'
    printf -- '- `absolute improvement = spectra best@k - baseline best@k`\n'
    printf -- '- `relative improvement = (spectra best@k - baseline best@k) / baseline best@k`\n'
    printf -- '- `pass@k = 1` if any candidate in that group with order <= k passes every test\n'
  } > "$summary"
}

generate_all_candidates() {
  local i
  local modality
  local round
  local candidate

  for ((i = 1; i <= CANDIDATES; i++)); do
    candidate="baseline_$i"
    generate_candidate "$candidate" "baseline" "baseline"
  done

  for ((i = 1; i <= CANDIDATES; i++)); do
    modality="$(candidate_modality_for_order "$i")"
    round=$(( (i - 1) / 3 + 1 ))
    candidate="spectra_${modality}_${round}"
    generate_candidate "$candidate" "spectra" "$modality"
  done
}

evaluate_all_candidates() {
  local i
  local modality
  local round
  local candidate

  printf 'group\tgroup_order\tcandidate\tmodality\tbuild_status\tpassed\ttotal\tscore\tfull_pass\n' > "$REPORTS_DIR/scores.tsv"
  printf 'candidate\ttest\tpass\tstdout_match\tstderr_match\tstatus_match\texpected_status\tactual_status\toutput_dir\n' > "$REPORTS_DIR/test-results.tsv"

  for ((i = 1; i <= CANDIDATES; i++)); do
    candidate="baseline_$i"
    score_candidate "$candidate" "baseline" "$i" "baseline"
  done

  for ((i = 1; i <= CANDIDATES; i++)); do
    modality="$(candidate_modality_for_order "$i")"
    round=$(( (i - 1) / 3 + 1 ))
    candidate="spectra_${modality}_${round}"
    score_candidate "$candidate" "spectra" "$i" "$modality"
  done
}

main() {
  info "run directory: $RUN_DIR"
  info "model: $MODEL"
  info "opencode timeout: ${OPENCODE_TIMEOUT}s"
  if [[ "$AUTO_APPROVE" -ne 1 ]]; then
    warn "opencode file writes may require approval; use --auto-approve for unattended candidate generation"
  fi

  prepare_tests
  generate_oracle_outputs

  if [[ -n "$EVALUATE_EXISTING" ]]; then
    info "evaluating existing generated packages only"
    evaluate_all_candidates
    write_summary
    info "summary: $REPORTS_DIR/summary.md"
    info "scores: $REPORTS_DIR/scores.tsv"
    return 0
  fi

  write_io_spec
  generate_specs
  generate_all_candidates
  evaluate_all_candidates
  write_summary

  info "summary: $REPORTS_DIR/summary.md"
  info "scores: $REPORTS_DIR/scores.tsv"
}

main "$@"
