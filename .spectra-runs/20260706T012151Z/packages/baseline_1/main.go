package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

type catState struct {
	lineNum         int
	newlines        int
	showNonprinting bool
	showTabs        bool
	number          bool
	numberNonblank  bool
	showEnds        bool
	squeezeBlank    bool
}

func main() {
	var (
		number          bool
		numberNonblank  bool
		squeezeBlank    bool
		showEnds        bool
		showNonprinting bool
		showTabs        bool
	)

	var files []string
	args := os.Args[1:]

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			files = append(files, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "--") {
			switch arg {
			case "--help":
				usage(0)
			case "--version":
				printVersion()
			case "--number-nonblank":
				numberNonblank = true
				number = true
			case "--number":
				number = true
			case "--squeeze-blank":
				squeezeBlank = true
			case "--show-nonprinting":
				showNonprinting = true
			case "--show-ends":
				showEnds = true
			case "--show-tabs":
				showTabs = true
			case "--show-all":
				showNonprinting = true
				showEnds = true
				showTabs = true
			default:
				fmt.Fprintf(os.Stderr, "%s: unrecognized option '%s'\n", os.Args[0], arg)
				usage(1)
			}
			continue
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			for _, c := range arg[1:] {
				switch c {
				case 'b':
					numberNonblank = true
					number = true
				case 'e':
					showEnds = true
					showNonprinting = true
				case 'n':
					number = true
				case 's':
					squeezeBlank = true
				case 't':
					showTabs = true
					showNonprinting = true
				case 'u':
					// ignored
				case 'v':
					showNonprinting = true
				case 'A':
					showNonprinting = true
					showEnds = true
					showTabs = true
				case 'E':
					showEnds = true
				case 'T':
					showTabs = true
				default:
					fmt.Fprintf(os.Stderr, "%s: invalid option -- '%c'\n", os.Args[0], c)
					usage(1)
				}
			}
			continue
		}
		files = append(files, arg)
		}

	if len(files) == 0 {
		files = []string{"-1"}
		files[0] = "-"
	}

	state := &catState{
		newlines:        0,
		showNonprinting: showNonprinting,
		showTabs:        showTabs,
		number:          number,
		numberNonblank:  numberNonblank,
		showEnds:        showEnds,
		squeezeBlank:    squeezeBlank,
	}

	out := bufio.NewWriter(os.Stdout)
	defer func() {
		if err := out.Flush(); err != nil {
			writeError(err)
		}
	}()

	ok := true
	for _, infile := range files {
		if !processFile(infile, out, state) {
			ok = false
		}
	}

	if !ok {
		os.Exit(1)
	}
}

func processFile(infile string, out *bufio.Writer, state *catState) bool {
	var f *os.File
	if infile == "-" {
		f = os.Stdin
	} else {
		var err error
		f, err = os.Open(infile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %s: %v\n", os.Args[0], infile, err)
			return false
		}
		defer f.Close()
	}

	if infile != "-" {
		inStat, err := f.Stat()
		if err == nil {
			outStat, err := os.Stdout.Stat()
			if err == nil {
				if os.SameFile(inStat, outStat) {
					fmt.Fprintf(os.Stderr, "%s: %s: input file is output file\n", os.Args[0], infile)
					return false
				}
			}
		}
	}

	if !state.number && !state.showEnds && !state.showNonprinting && !state.showTabs && !state.squeezeBlank {
		_, err := io.Copy(out, f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %s: %v\n", os.Args[0], infile, err)
			return false
		}
		return true
	}

	r := bufio.NewReader(f)
	return cat(r, out, state, infile)
}

func cat(r *bufio.Reader, w *bufio.Writer, state *catState, infile string) bool {
	for {
		ch, err := r.ReadByte()
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Fprintf(os.Stderr, "%s: %s: %v\n", os.Args[0], infile, err)
			return false
		}

		if ch == '\n' {
			state.newlines++
			if state.newlines >= 2 {
				state.newlines = 2
				if state.squeezeBlank {
					continue
				}
			}
			if state.number && !state.numberNonblank {
				state.lineNum++
				if _, err := fmt.Fprintf(w, "%6d\t", state.lineNum); err != nil {
					writeError(err)
				}
			}
			if state.showEnds {
				if err := w.WriteByte('$'); err != nil {
					writeError(err)
				}
			}
			if err := w.WriteByte('\n'); err != nil {
				writeError(err)
			}
		} else {
			if state.newlines >= 0 {
				if state.number {
					state.lineNum++
					if _, err := fmt.Fprintf(w, "%6d\t", state.lineNum); err != nil {
						writeError(err)
					}
				}
			}
			state.newlines = -1

			nextLf := false
			if ch == '\r' && state.showEnds && !state.showNonprinting {
				peekBytes, err := r.Peek(1)
				if err == nil && len(peekBytes) > 0 && peekBytes[0] == '\n' {
					nextLf = true
				}
			}

			if err := writeFormattedChar(w, ch, nextLf, state); err != nil {
				writeError(err)
			}
		}
	}
	return true
}

func writeFormattedChar(w *bufio.Writer, ch byte, nextLf bool, state *catState) error {
	if state.showNonprinting {
		if ch >= 32 {
			if ch < 127 {
				return w.WriteByte(ch)
			} else if ch == 127 {
				_, err := w.WriteString("^?")
				return err
			} else {
				_, err := w.WriteString("M-")
				if err != nil {
					return err
				}
				if ch >= 160 {
					if ch < 255 {
						return w.WriteByte(ch - 128)
					} else {
						_, err := w.WriteString("^?")
						return err
					}
				} else {
					err := w.WriteByte('^')
					if err != nil {
						return err
					}
					return w.WriteByte(ch - 64)
				}
			}
		} else {
			if ch == '\t' && !state.showTabs {
				return w.WriteByte('\t')
			} else {
				err := w.WriteByte('^')
				if err != nil {
						return err
				}
				return w.WriteByte(ch + 64)
			}
		}
	} else {
		if ch == '\t' && state.showTabs {
			_, err := w.WriteString("^I")
			return err
		}
		if ch == '\r' && nextLf && state.showEnds {
			_, err := w.WriteString("^M")
			return err
		}
		return w.WriteByte(ch)
	}
}

func writeError(err error) {
	fmt.Fprintf(os.Stderr, "%s: write error: %v\n", os.Args[0], err)
	os.Exit(1)
}

func usage(status int) {
	out := os.Stdout
	if status != 0 {
		out = os.Stderr
		fmt.Fprintln(out, "Try 'cat --help' for more information.")
		os.Exit(status)
	}
	fmt.Fprintf(out, "Usage: %s [OPTION]... [FILE]...\n", os.Args[0])
	fmt.Fprintln(out, "Concatenate FILE(s) to standard output.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "With no FILE, or when FILE is -, read standard input.")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  -A, --show-all           equivalent to -vET")
	fmt.Fprintln(out, "  -b, --number-nonblank    number nonempty output lines, overrides -n")
	fmt.Fprintln(out, "  -e                       equivalent to -vE")
	fmt.Fprintln(out, "  -E, --show-ends          display $ or ^M$ at end of each line")
	fmt.Fprintln(out, "  -n, --number             number all output lines")
	fmt.Fprintln(out, "  -s, --squeeze-blank      suppress repeated empty output lines")
	fmt.Fprintln(out, "  -t                       equivalent to -vT")
	fmt.Fprintln(out, "  -T, --show-tabs          display TAB characters as ^I")
	fmt.Fprintln(out, "  -u                       (ignored)")
	fmt.Fprintln(out, "  -v, --show-nonprinting   use ^ and M- notation, except for LFD and TAB")
	fmt.Fprintln(out, "      --help      display this help and exit")
	fmt.Fprintln(out, "      --version   output version information and exit")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Examples:")
	fmt.Fprintf(out, "  %s f - g  Output f's contents, then standard input, then g's contents.\n", os.Args[0])
	fmt.Fprintf(out, "  %s        Copy standard input to standard output.\n", os.Args[0])
	os.Exit(status)
}

func printVersion() {
	fmt.Println("cat (GNU coreutils) 9.5 (translated to Go)")
	fmt.Println("Copyright (C) 2026 Free Software Foundation, Inc.")
	fmt.Println("License GPLv3+: GNU GPL version 3 or later <https://gnu.org/licenses/gpl.html>.")
	fmt.Println("This is free software: you are free to change and redistribute it.")
	fmt.Println("There is NO WARRANTY, to the extent permitted by law.")
	fmt.Println()
	fmt.Println("Written by Torbjorn Granlund and Richard M. Stallman.")
	os.Exit(0)
}
