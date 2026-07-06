## Program input/output contract

- Inputs are `FILE...` operands plus options. Each operand is either a pathname or `-`, where `-` means standard input.
- If no operands remain after option parsing, the program processes a single implicit operand `-`.
- Output is the concatenation of each input stream in operand order, written to standard output.
- In unformatted mode, bytes are copied as-is.
- In formatted mode, output may differ from input only by:
  - inserting line numbers and a trailing tab,
  - suppressing repeated empty output lines,
  - rendering tabs, line ends, carriage returns before line feeds, and nonprinting bytes with visible notation.
- Exit status is success iff every requested input was processed without a diagnosed read/open/stat/write/copy failure.
- Processing continues across later operands after a per-file open/stat/read failure when possible.
- Standard input may be consumed multiple times if `-` appears multiple times.

## Option parsing contract

- Recognized short options: `-A -b -e -E -n -s -t -T -u -v` plus help/version.
- Recognized long options: `--show-all --number-nonblank --number --squeeze-blank --show-nonprinting --show-ends --show-tabs` plus help/version.
- Option effects are cumulative except where explicitly overridden.
- `-b` sets `number = true` and `number_nonblank = true`.
- `-n` sets `number = true`.
- `-b` overrides `-n` on empty lines only: nonempty lines are numbered either way; empty lines are numbered only when `number = true` and `number_nonblank = false`.
- `-s` enables suppression of repeated adjacent empty output lines.
- `-E` enables visible line-end markers.
- `-T` enables visible tab markers.
- `-v` enables visible rendering of nonprinting bytes, excluding LF and excluding TAB unless `show_tabs` is also enabled.
- `-u` has no behavioral effect and must be accepted silently.
- Presence of any formatting option other than `-u` selects formatted processing; otherwise plain copying is used.

## Preconditions and postconditions for `simple_cat` behavior

**Preconditions**

- `input_desc` is an open readable file descriptor for the current input.
- `buf` points to writable storage of at least `bufsize` bytes.
- Standard output is available for writing.

**Postconditions**

- On success, every byte read from `input_desc` until EOF is written to standard output in the same order and with identical values.
- No byte transformation, insertion, or suppression occurs.
- Return value is `true` on EOF after successful copying.
- Return value is `false` on read failure after reporting the input-file error.
- Any short or failed final write is treated as a fatal write error, not a `false` return.
- File position after success is at EOF for seekable inputs, otherwise advanced by exactly the consumed bytes.

## Preconditions and postconditions for formatted `cat` behavior

**Preconditions**

- `input_desc` is an open readable file descriptor for the current input.
- `inbuf` has capacity at least `insize + 1`; the extra byte is reserved for a newline sentinel.
- `outbuf` has enough capacity to hold worst-case expansion for one input block plus any buffered remainder and one line-number string.
- `newlines2` and `pending_cr` carry state from the previous operand and must be preserved across operands.
- Standard output is available for writing.

**Postconditions**

- On success, the entire input is emitted after applying the active formatting transformations exactly once in stream order.
- `newlines2` is updated to the final newline-state of the processed input so numbering and blank squeezing continue correctly across the next operand.
- `pending_cr` is updated so a trailing `\r` at an input-buffer boundary is either rendered before a following `\n` as `^M` when `show_ends` is active, or emitted literally later.
- Output flushing preserves byte order and does not duplicate or skip transformed bytes.
- Return value is `true` on EOF after successful formatted emission.
- Return value is `false` on diagnosed read/ioctl failure after flushing already-buffered output.
- Any failed write is fatal rather than represented by `false`.

## Invariants

### Line numbering

- A line is numbered only at logical line start.
- Logical line start exists initially, after each consumed newline, and across file boundaries according to `newlines2`.
- If `number_nonblank = false`, every output line start is numbered, including empty lines that survive squeezing.
- If `number_nonblank = true`, only nonempty lines are numbered.
- Each printed line number is strictly increasing by 1 and is emitted before any content of that numbered line.
- Printed number format is right-aligned decimal digits followed by `\t`; overflow extends leftward and eventually replaces the leftmost cell with `>`.

### Squeeze blank

- An empty line is a line whose content before newline is empty.
- Without `-s`, every input newline that terminates an empty line produces a corresponding output line.
- With `-s`, among any maximal run of adjacent empty lines, exactly one empty output line is emitted.
- The squeeze decision is based on logical line structure and continues across file boundaries via `newlines2`.

### Show tabs

- If `show_tabs = false`, TAB is preserved as byte `0x09` unless transformed by some other rule that does not apply here.
- If `show_tabs = true`, each TAB is rendered as the two-byte sequence `^I`.
- Under `show_nonprinting`, TAB still remains raw unless `show_tabs = true`.

### Show ends

- If `show_ends = true`, each emitted newline is immediately preceded by `$`.
- If the input contains `\r\n` and `show_ends = true` in the non-quoting path, the `\r` is rendered as `^M` before `$` and `\n`.
- A `\r` split across buffer or file-end state is preserved with `pending_cr` so the same visible result is produced as if processed contiguously.
- If `show_ends = false`, no `$` markers are inserted.

### Show nonprinting

- If `show_nonprinting = true`, bytes are rendered using caret/meta notation except LF, and except TAB when `show_tabs = false`.
- ASCII control bytes other than LF/TAB render as `^X` where `X = byte + 64`; DEL renders as `^?`.
- Bytes `128..255` render with `M-` prefix plus the notation of `byte - 128`.
- Printable ASCII bytes `32..126` render unchanged.
- If `show_nonprinting = false`, bytes other than TAB/CR special cases pass through unchanged.

## Explicit equivalences

- `-A` is exactly equivalent to enabling `-v`, `-E`, and `-T`.
- `-e` is exactly equivalent to enabling `-v` and `-E`.
- `-t` is exactly equivalent to enabling `-v` and `-T`.
- `-u` is accepted and ignored; behavior is identical whether `-u` is present or absent.