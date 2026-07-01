# Program Input/Output Contract

- Reads zero or more input files named on the command line.
- A file name of `-` means standard input.
- Writes concatenated output to standard output.
- Processes files in argv order.
- Returns success iff all inputs were processed without fatal read/open/write errors.
- Never buffers user-visible output semantics differently based on `-u`; output is effectively unbuffered in the C implementation, but translation may use ordinary buffering if behavior is preserved.

# Option Parsing Contract

- Recognize short options: `-A -b -e -E -n -s -t -u -v -T`.
- Recognize long options:
  - `--show-all`
  - `--number-nonblank`
  - `--show-ends`
  - `--number`
  - `--squeeze-blank`
  - `--show-nonprinting`
  - `--show-tabs`
- Unknown options are an error and must trigger usage failure.
- Option interactions:
  - `-b` enables line numbering and overrides `-n` for nonblank numbering.
  - `-e` implies `-v` and `-E`.
  - `-t` implies `-v` and `-T`.
  - `-A` implies `-v`, `-E`, and `-T`.
  - `-u` is accepted but ignored.

# Preconditions and Postconditions for `simple_cat` Behavior

## Preconditions
- Input is in plain copy mode: no numbering, no visible-control-character expansion, no end-of-line markers, no blank-line squeezing.
- Input descriptor is open and readable.
- Output descriptor is writable.
- Translation may assume a reusable byte buffer is available.

## Postconditions
- Copies bytes from input to output unchanged.
- Preserves all byte values exactly, including `NUL`, `CR`, and `LF`.
- Stops at EOF.
- On read error: reports failure for that input and returns false.
- On write failure: reports write error and terminates that attempt as failure.

# Preconditions and Postconditions for Formatted `cat` Behavior

## Preconditions
- At least one of the formatting options is active: `-n`, `-b`, `-s`, `-v`, `-E`, or `-T`.
- Input descriptor is open and readable.
- Output descriptor is writable.
- Translation should maintain state across files:
  - line-number state
  - pending CR state
  - newline run state

## Postconditions
- Output is a transformed stream, not a byte-for-byte copy.
- Line numbering, blank squeezing, tab display, end-of-line display, and nonprinting expansion are applied according to active options.
- State persists across input files exactly as if files were concatenated into one stream.
- On read/write error, preserve and return the current state as needed for subsequent files, but report failure.

# Invariants

## Line Numbering
- Line numbers start at `1`.
- The number field is right-aligned in a fixed-width, space-padded column followed by a tab.
- `-n` numbers every output line.
- `-b` numbers only nonblank output lines and suppresses `-n` for blank lines.
- A line is considered blank if its content is only `\n`.
- Line numbers increment only when an output line is actually numbered.
- Line-number formatting must remain stable even after large counts; output width may expand if needed.

## Squeeze Blank
- `-s` suppresses repeated empty output lines.
- At most one consecutive blank line may be emitted.
- State must persist across files, so a blank line at the end of one file can suppress the first blank line of the next file.

## Show Tabs
- `-T` renders TAB as `^I`.
- When `-t` or `-A` is active, tabs are visible.
- If `-v` is active without `-T`, TAB remains a literal tab.

## Show Ends
- `-E` appends `$` before each newline output.
- When a carriage return is immediately before a newline and `-E` is active, output `^M$` for that CRLF sequence.
- A pending CR at buffer boundaries must be remembered so the `^M` is emitted correctly when the following `\n` arrives.
- If a file ends with a pending CR, it must be flushed literally at the end.

## Show Nonprinting
- `-v` expands nonprinting bytes using caret/M- notation.
- Printable ASCII bytes `32..126` pass through unchanged.
- `DEL` (`127`) becomes `^?`.
- Bytes `128..255` become `M-...` forms, with nested caret notation where needed.
- TAB and newline are special:
  - TAB is literal unless `-T` is active.
  - newline is never quoted as a control character; it terminates the current line logic.
- `-v` does not alter ordinary printable characters.

# Explicit Equivalences

- `-A` is equivalent to `-vET`.
- `-e` is equivalent to `-vE`.
- `-t` is equivalent to `-vT`.
- `-u` is ignored and must not change output behavior.
