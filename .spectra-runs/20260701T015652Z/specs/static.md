## Program Input/Output Contract
- Reads zero or more path arguments; `-` means standard input.
- Writes concatenated input to standard output.
- Returns success iff every input source was processed and closed successfully.
- On any open/read/write/close failure, reports an error and exits failure.
- If no formatting options are active, output is copied byte-for-byte, preserving binary data.
- Must not copy a regular file onto itself when source and destination are the same inode/position-sensitive target.

## Option Parsing Contract
- Recognize `-b`, `-e`, `-n`, `-s`, `-t`, `-u`, `-v`, `-A`, `-E`, `-T`, plus long forms shown in usage.
- `-b` enables line numbering of nonblank output lines and overrides `-n`.
- `-n` enables numbering of all output lines.
- `-s` squeezes repeated blank output lines.
- `-e` implies `-v` and `-E`.
- `-t` implies `-v` and `-T`.
- `-A` implies `-v`, `-E`, and `-T`.
- `-u` has no effect.
- If no formatting options are set, use the fast path: `copy_cat` first, then `splice_cat`, then `simple_cat`.

## Preconditions and Postconditions for `simple_cat`
### Preconditions
- Input descriptor is valid and open for reading.
- Output descriptor is standard output.
- Buffer is allocated to at least the requested size.
- No formatting option requiring transformation is active.

### Postconditions
- All readable bytes from input are written unchanged to stdout in order.
- Returns `true` only if EOF is reached without read/write error.
- On read error, emits the input error and returns `false`.
- On short/full write failure, terminates via write error handling.

## Preconditions and Postconditions for Formatted `cat`
### Preconditions
- Input and output descriptors are valid.
- Input buffer is at least `insize + 1`.
- Output buffer is large enough for worst-case expansion:
  `outsize - 1 + 4*insize + LINE_COUNTER_BUF_LEN - 1`.
- A newline sentinel is placed at the end of the input buffer each refill.

### Postconditions
- Produces transformed output according to active flags:
  - line numbering
  - blank-line squeezing
  - tab display
  - end-of-line marking
  - nonprinting character quoting
- Preserves input order and stream boundaries across files.
- Returns `true` only if all input bytes were consumed and emitted successfully.
- Stores pending state (`newlines2`, `pending_cr`) so the next invocation continues correctly.

## Invariants
### Line Numbering
- `newlines >= 0` means the next non-newline character begins a new output line.
- `newlines < 0` means currently inside a line body.
- Line numbers are emitted only at start-of-line positions.
- `-b` numbers only nonblank lines; `-n` numbers all lines; `-b` dominates `-n`.
- The line counter is incremented exactly once per numbered output line.
- The printed counter string grows leftward as needed and is always NUL-terminated.

### Squeeze Blank
- Consecutive blank output lines are counted by consecutive newline input runs.
- When `-s` is active, the second and later empty output lines in a run are suppressed.
- Blank-line suppression applies only to actual newline output, not to transformed character sequences.

### Show Tabs
- When `-T` or `-A` or `-t` is active, tab characters are shown as `^I`.
- When show-tabs is inactive, tab characters are passed through unchanged unless other quoting rules apply.

### Show Ends
- When `-E`, `-A`, or `-e` is active, each output line ends with `$` before the newline.
- In CRLF cases, a pending `\r` may be rendered as `^M` before `$`.
- End markers are emitted once per physical newline boundary.

### Show Nonprinting
- When `-v`, `-e`, `-t`, or `-A` is active, nonprinting bytes are quoted.
- Printable ASCII `32..126` pass through unchanged, except `DEL` becomes `^?`.
- Bytes `>= 128` are rendered with `M-` notation, possibly followed by `^X` or a printable byte.
- `TAB` is exempt from quoting unless tab-display is enabled.
- `NEWLINE` is never quoted; it terminates the current line state.

## Explicit Equivalences
- `-A` == `-vET`
- `-e` == `-vE`
- `-t` == `-vT`
- `-u` is ignored and must not change behavior
