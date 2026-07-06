## Program input/output contract

- Invocation: `cat [OPTION]... [FILE]...`.
- Process operands from left to right. `-` denotes standard input; if no files are supplied, read standard input once.
- Write the logical concatenation to standard output, applying one combined option set across all operands.
- Formatting state—including line number, preceding newline count, and a pending carriage return—persists across file boundaries.
- Report open, read, metadata, close, and same-file errors to standard error; continue with later operands when possible.
- A standard-output write failure is fatal.
- Exit successfully only if every input operation completed successfully.
- Reject copying a seekable input onto a later position of the same output file when that could grow the file indefinitely.
- Treat data as bytes; locale must not alter transformations.

## Option parsing contract

| Option | Effect |
|---|---|
| `-n`, `--number` | Number every output line. |
| `-b`, `--number-nonblank` | Number nonempty lines and permanently override `-n`, independent of option order. |
| `-s`, `--squeeze-blank` | Replace each run of adjacent empty lines with one empty line. |
| `-E`, `--show-ends` | Insert `$` immediately before each LF; represent CR immediately before LF as `^M` before `$`. |
| `-T`, `--show-tabs` | Represent TAB as `^I`. |
| `-v`, `--show-nonprinting` | Render nonprinting bytes except LF and, unless `-T` is active, TAB. |
| `-u` | Accepted with no semantic effect. |
| `--help`, `--version` | Print the requested information and exit successfully. |

Unknown options or missing required syntax produce diagnostics and a failing exit.

Use the formatted path if any of `-n`, `-b`, `-s`, `-E`, `-T`, or `-v` is effective. Otherwise use the simple copy path.

## Preconditions and postconditions for `simple_cat`

### Preconditions

- The input and standard-output streams are open.
- The buffer is writable and has positive capacity.
- No formatting option other than ignored `-u` is active.

### Postconditions

- On success, every byte read from the current input is written once, unchanged and in order.
- Reads continue until EOF.
- An input read error is diagnosed and returns failure; bytes already written remain written.
- Partial writes are retried until complete; an unrecoverable output error terminates the program.

## Preconditions and postconditions for formatted `cat`

### Preconditions

- Input capacity is positive and includes one additional byte for an LF sentinel.
- Output storage can hold retained output plus the maximum expansion of one input block: four bytes per input byte and one line-number prefix.
- Formatting state from prior operands is supplied unchanged.

### Postconditions

- All successfully read bytes are transformed in order according to the active options.
- The sentinel is never emitted or treated as actual input.
- Buffered output is flushed before EOF, return after a read error, or a potentially blocking read.
- On EOF, return success and preserve cross-file formatting state.
- On input error, diagnose it, flush already transformed output, preserve state, and return failure.
- A trailing pending CR is emitted literally after all operands if no following LF resolves it.

## Invariants

### Line numbering

- The counter starts at zero and increments immediately before each emitted prefix.
- A prefix is decimal line number right-aligned to at least six columns, followed by TAB.
- `-n` prefixes every retained line, including empty lines.
- `-b` prefixes only lines containing at least one byte before LF; it suppresses `-n` numbering of empty lines.
- A final non-LF-terminated nonempty line is numbered.
- The counter and beginning-of-line state persist across buffers and files.

### Squeeze blank

- An empty line is an LF encountered while already at the beginning of a line.
- For every maximal run of two or more consecutive LF bytes, emit at most two LF bytes: one terminates the preceding line and one represents a single empty line.
- Suppressed empty lines receive neither a number nor an end marker.
- Run detection persists across buffer and file boundaries.

### Show tabs

- With `-T`, every TAB becomes the two bytes `^I`.
- Without `-T`, TAB remains literal, including when `-v` is active.

### Show ends

- Every retained LF is emitted as `$` followed by LF.
- A CR directly preceding LF is rendered as `^M$` followed by LF, including when CR and LF occur in different buffers or files.
- A CR not followed by LF remains literal unless `-v` converts it.

### Show nonprinting

For each byte other than LF and an exempt literal TAB:

- `0x00..0x1F` → `^` followed by byte plus `0x40`.
- `0x20..0x7E` → unchanged.
- `0x7F` → `^?`.
- `0x80..0x9F` → `M-^` followed by byte minus `0x80`, plus `0x40`.
- `0xA0..0xFE` → `M-` followed by byte minus `0x80`.
- `0xFF` → `M-^?`.

Transformations do not change whether an input LF defines a line boundary.

## Explicit option equivalences

- `-A` ≡ `-v -E -T`.
- `-e` ≡ `-v -E`.
- `-t` ≡ `-v -T`.
- `-u` ≡ no option: it changes neither buffering guarantees nor output.