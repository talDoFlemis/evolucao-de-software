package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

var (
	number          bool
	numberNonblank  bool
	squeezeBlank    bool
	showEnds        bool
	showNonprinting bool
	showTabs        bool
)

type catState struct {
	newlines      int
	atStartOfLine bool
	lineNum       uint64
}

func main() { 
	files := []string{}
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "--" {
			files = append(files, os.Args[i+1:]...)
			break
		}
		if arg == "-" {
			files = append(files, "-")
			continue
		}
		if strings.HasPrefix(arg, "--") {
			switch arg {
			case "--number-nonblank":
				number = true
				numberNonblank = true
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
			case "--help":
				printHelp()
				os.Exit(0)
			case "--version":
				printVersion()
				os.Exit(0)
			default:
				fmt.Fprintf(os.Stderr, "cat: unrecognized option '%s'\n", arg)
				fmt.Fprintf(os.Stderr, "Try 'cat --help' for more information.\n")
				os.Exit(1)
			}
		} else if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			for _, ch := range arg[1:] {
				switch ch {
				case 'b':
					number = true
					numberNonblank = true
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
					fmt.Fprintf(os.Stderr, "cat: invalid option -- '%c'\n", ch)
					fmt.Fprintf(os.Stderr, "Try 'cat --help' for more information.\n")
					os.Exit(1)
				}
			}
		} else {
			files = append(files, arg)
		}
	}

	state := &catState{
		newlines:      0,
		atStartOfLine: true,
		lineNum:       0,
	}

	ok := true
	if len(files) == 0 {
		if !processFile("-", state) {
			ok = false
		}
	} else {
		for _, filename := range files {
			if !processFile(filename, state) {
				ok = false
			}
		}
	}

	if !ok {
		os.Exit(1)
	}
}

func printHelp() { 
	fmt.Printf("Usage: %s [OPTION]... [FILE]...\n", os.Args[0])
	fmt.Println("Concatenate FILE(s) to standard output.")
	fmt.Println("\nWith no FILE, or when FILE is -, read standard input.")
	fmt.Println()
	fmt.Println("  -A, --show-all           equivalent to -vET")
	fmt.Println("  -b, --number-nonblank    number nonempty output lines, overrides -n")
	fmt.Println("  -e                       equivalent to -vE")
	fmt.Println("  -E, --show-ends          display $ at end of each line")
	fmt.Println("  -n, --number             number all output lines")
	fmt.Println("  -s, --squeeze-blank      suppress repeated empty output lines")
	fmt.Println("  -t                       equivalent to -vT")
	fmt.Println("  -T, --show-tabs          display TAB characters as ^I")
	fmt.Println("  -u                       (ignored)")
	fmt.Println("  -v, --show-nonprinting   use ^ and M- notation, except for LFD and TAB")
	fmt.Println("      --help     display this help and exit")
	fmt.Println("      --version  output version information and exit")
	fmt.Println("\nExamples:")
	fmt.Printf("  %s f - g  Output f's contents, then standard input, then g's contents.\n", os.Args[0])
	fmt.Printf("  %s        Copy standard input to standard output.\n", os.Args[0])
}

func printVersion() { 
	fmt.Println("cat (Go translation) 1.0")
	fmt.Println("Copyright (C) 2026 Free Software Foundation, Inc.")
	fmt.Println("License GPLv3+: GNU GPL version 3 or later <https://gnu.org/licenses/gpl.html>.")
	fmt.Println("This is free software: you are free to change and redistribute.")
	fmt.Println("There is NO WARRANTY, to the extent permitted by law.")
	fmt.Println()
	fmt.Println("Written by Torbjörn Granlund and Richard M. Stallman (translated to Go).")
}

func formatLineNum(lineNum *uint64) []byte {
	*lineNum++
	var buf [24]byte
	i := len(buf) - 1
	buf[i] = '\t'
	i--

	temp := *lineNum
	for temp > 0 {
		buf[i] = byte('0' + (temp % 10))
		temp /= 10
		i--
	}

	digits := (len(buf) - 2) - i
	for digits < 6 {
		buf[i] = ' '
		i--
		digits++
	}

	res := make([]byte, len(buf)-1-i)
	copy(res, buf[i+1:])
	return res
}

func simpleCat(infile string, r io.Reader) bool {
	buf := make([]byte, 64*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if _, werr := os.Stdout.Write(buf[:n]); werr != nil {
				fmt.Fprintf(os.Stderr, "cat: write error: %v\n", werr)
				os.Exit(1)
			}
		}
		if err != nil {
			if err == io.EOF {
				return true
			}
			fmt.Fprintf(os.Stderr, "cat: %s: %v\n", infile, err)
			return false
		}
	}
}

func writeNonprinting(w *bufio.Writer, ch byte, showTabs bool) { 
	if ch >= 32 {
		if ch < 127 {
			w.WriteByte(ch)
		} else if ch == 127 {
			w.Write([]byte("^?"))
		} else {
			w.Write([]byte("M-"))
			if ch >= 128+32 {
				if ch < 128+127 {
					w.WriteByte(ch - 128)
				} else {
					w.Write([]byte("^?"))
				}
			} else {
				w.WriteByte('^')
				w.WriteByte(ch - 128 + 64)
			}
		}
	} else if ch == '\t' && !showTabs {
		w.WriteByte('\t')
	} else {
		w.WriteByte('^')
		w.WriteByte(ch + 64)
	}
}

func catFormatted(infile string, r *bufio.Reader, w *bufio.Writer, state *catState) bool {
	for {
		ch, err := r.ReadByte()
		if err != nil {
			if err == io.EOF {
				return true
			}
			fmt.Fprintf(os.Stderr, "cat: %s: %v\n", infile, err)
			return false
		}

		if ch == '\n' {
			if state.atStartOfLine {
				state.newlines++
				if state.newlines >= 2 {
					if squeezeBlank {
						continue
					}
				}
				if number && !numberNonblank {
					w.Write(formatLineNum(&state.lineNum))
				}
				if showEnds {
					w.WriteByte('$')
				}
				w.WriteByte('\n')
			} else {
				state.newlines = 0
				if showEnds {
					w.WriteByte('$')
				}
				w.WriteByte('\n')
				state.atStartOfLine = true
			}
		} else {
			// Non-newline character
			if state.atStartOfLine {
				state.newlines = -1
				if number {
					w.Write(formatLineNum(&state.lineNum))
				}
				state.atStartOfLine = false
			}

			// Process the character ch
			if showNonprinting {
				writeNonprinting(w, ch, showTabs)
			} else {
				if ch == '\t' && showTabs {
					w.Write([]byte("^I"))
				} else if ch == '\r' && showEnds {
					// Check if next byte is '\n'
					next, peekErr := r.Peek(1)
					if peekErr == nil && next[0] == '\n' {
						w.Write([]byte("^M"))
					} else {
						w.WriteByte('\r')
					}
				} else {
					w.WriteByte(ch)
				}
			}
		}
	}
}

func processFile(filename string, state *catState) bool {
	var f *os.File
	var err error
	if filename == "-" {
		f = os.Stdin
	} else {
		f, err = os.Open(filename)
		if err != nil {
			if pe, ok := err.(*os.PathError); ok {
				fmt.Fprintf(os.Stderr, "cat: %s: %s\n", filename, pe.Err.Error())
			} else {
				fmt.Fprintf(os.Stderr, "cat: %s: %v\n", filename, err)
			}
			return false
		}
		defer f.Close()

		info, err := f.Stat()
		if err != nil {
			fmt.Fprintf(os.Stderr, "cat: %s: %v\n", filename, err)
			return false
		}
		if info.IsDir() {
			fmt.Fprintf(os.Stderr, "cat: %s: Is a directory\n", filename)
			return false
		}
	}

	if !(number || showEnds || showNonprinting || showTabs || squeezeBlank) {
		return simpleCat(filename, f)
	}

	r := bufio.NewReader(f)
	w := bufio.NewWriter(os.Stdout)

	ok := catFormatted(filename, r, w, state)
	if err := w.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "cat: write error: %v\n", err)
		return false
	}
	return ok
}
