package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

type catState struct {
	newlines  int
	pendingCR bool
	lineCount int
}

type flushingReader struct {
	r io.Reader
	w *bufio.Writer
}

func (fr flushingReader) Read(p []byte) (int, error) {
	if err := fr.w.Flush(); err != nil {
		return 0, err
	}
	return fr.r.Read(p)
}

func printTryHelp() {
	fmt.Fprintf(os.Stderr, "Try '%s --help' for more information.\n", os.Args[0])
}

func printHelp() {
	fmt.Printf("Usage: %s [OPTION]... [FILE]...\n", os.Args[0])
	fmt.Println("Concatenate FILE(s) to standard output.")
	fmt.Println()
	fmt.Println("With no FILE, or when FILE is -, read standard input.")
	fmt.Println()
	fmt.Println("  -A, --show-all           equivalent to -vET")
	fmt.Println("  -b, --number-nonblank    number nonempty output lines, overrides -n")
	fmt.Println("  -e                       equivalent to -vE")
	fmt.Println("  -E, --show-ends          display $ or ^M$ at end of each line")
	fmt.Println("  -n, --number             number all output lines")
	fmt.Println("  -s, --squeeze-blank      suppress repeated empty output lines")
	fmt.Println("  -t                       equivalent to -vT")
	fmt.Println("  -T, --show-tabs          display TAB characters as ^I")
	fmt.Println("  -u                       (ignored)")
	fmt.Println("  -v, --show-nonprinting   use ^ and M- notation, except for LFD and TAB")
	fmt.Println("      --help     display this help and exit")
	fmt.Println("      --version  output version information and exit")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Printf("  %s f - g  Output f's contents, then standard input, then g's contents.\n", os.Args[0])
	fmt.Printf("  %s        Copy standard input to standard output.\n", os.Args[0])
}

func printVersion() {
	fmt.Println("cat (GNU coreutils) Go translation baseline")
	fmt.Println("Copyright (C) 2026 Free Software Foundation, Inc.")
	fmt.Println("License GPLv3+: GNU GPL version 3 or later <https://gnu.org/licenses/gpl.html>.")
	fmt.Println("This is free software: you are free to change and redistribute.  There is NO WARRANTY.")
	fmt.Println()
	fmt.Println("Written by Torbjorn Granlund and Richard M. Stallman.")
}

func writeLineNumber(w *bufio.Writer, line int) error { 
	var buf [32]byte
	pos := len(buf) - 1
	buf[pos] = '\t'
	pos--

	val := line
	count := 0
	for val > 0 {
		buf[pos] = byte('0' + (val % 10))
		val /= 10
		pos--
		count++
	}

	for count < 6 {
		buf[pos] = ' '
		pos--
		count++
	}

	_, err := w.Write(buf[pos+1:])
	return err
}

func writeChar(w *bufio.Writer, ch byte, showNonprinting, showTabs bool) error {
	if showNonprinting {
		if ch >= 32 {
			if ch < 127 {
				return w.WriteByte(ch)
			} else if ch == 127 {
				_, err := w.Write([]byte{'^', '?'})
				return err
			} else {
				if ch >= 128+32 {
					if ch < 128+127 {
						_, err := w.Write([]byte{'M', '-', ch - 128})
						return err
					} else {
						_, err := w.Write([]byte{'M', '-', '^', '?'})
						return err
					}
				} else {
					_, err := w.Write([]byte{'M', '-', '^', ch - 128 + 64})
					return err
				}
			}
		} else if ch == '\t' && !showTabs {
			return w.WriteByte('\t')
		} else {
			_, err := w.Write([]byte{'^', ch + 64})
			return err
		}
	} else {
		if ch == '\t' && showTabs {
			_, err := w.Write([]byte{'^', 'I'})
			return err
		} else {
			return w.WriteByte(ch)
		}
	}
}

func (s *catState) cat(r *bufio.Reader, w *bufio.Writer, showNonprinting, showTabs, number, numberNonblank, showEnds, squeezeBlank bool) error {
	for {
		ch, err := r.ReadByte()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if ch == '\n' {
			s.newlines++
			if s.newlines >= 2 {
				s.newlines = 2
				if squeezeBlank {
					continue
				}
			}

			if number && !numberNonblank {
				s.lineCount++
				if err := writeLineNumber(w, s.lineCount); err != nil {
					return err
				}
			}

			if showEnds {
				if s.pendingCR {
					if _, err := w.Write([]byte{'^', 'M'}); err != nil {
						return err
					}
					s.pendingCR = false
				}
				if _, err := w.Write([]byte{'$'}); err != nil {
					return err
				}
			}

			if err := w.WriteByte('\n'); err != nil {
				return err
			}
		} else {
			if s.pendingCR {
				if _, err := w.Write([]byte{'\r'}); err != nil {
					return err
				}
				s.pendingCR = false
			}

			if s.newlines >= 0 && number {
				s.lineCount++
				if err := writeLineNumber(w, s.lineCount); err != nil {
					return err
				}
			}
			s.newlines = -1

			if ch == '\r' && showEnds && !showNonprinting {
				next, err := r.Peek(1)
				if err == nil && next[0] == '\n' {
					if _, err := w.Write([]byte{'^', 'M'}); err != nil {
						return err
					}
					continue
				}
				if err == io.EOF || (err != nil && err != bufio.ErrBufferFull) {
					s.pendingCR = true
					continue
				}
			}

			if err := writeChar(w, ch, showNonprinting, showTabs); err != nil {
				return err
			}
		}
	}
	return nil
}

func simpleCat(r io.Reader, w io.Writer, filename string) bool { 
	buf := make([]byte, 64*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			_, werr := w.Write(buf[:n])
			if werr != nil {
				fmt.Fprintf(os.Stderr, "cat: write error: %v\n", werr)
				os.Exit(1)
			}
		}
		if err != nil {
			if err == io.EOF {
				return true
			}
			fmt.Fprintf(os.Stderr, "cat: %s: %v\n", filename, err)
			return false
		}
	}
}

func main() {
	var files []string
	var number, numberNonblank, squeezeBlank, showEnds, showNonprinting, showTabs bool
	var help, version bool

	stopFlags := false
	args := os.Args[1:]
	for i := 0; i < len(args); i++ { 
		arg := args[i]
		if stopFlags {
			files = append(files, arg)
			continue
		}
		if arg == "--" {
			stopFlags = true
			continue
		}
		if arg == "-" {
			files = append(files, arg)
			continue
		}
		if strings.HasPrefix(arg, "--") {
			opt := arg[2:]
			switch opt {
			case "number-nonblank":
				number = true
				numberNonblank = true
			case "number":
				number = true
			case "squeeze-blank":
				squeezeBlank = true
			case "show-nonprinting":
				showNonprinting = true
			case "show-ends":
				showEnds = true
			case "show-tabs":
				showTabs = true
			case "show-all":
				showNonprinting = true
				showEnds = true
				showTabs = true
			case "help":
				help = true
			case "version":
				version = true
			default:
				fmt.Fprintf(os.Stderr, "cat: unrecognized option '%s'\n", arg)
				printTryHelp()
				os.Exit(1)
			}
			continue
		}
		if strings.HasPrefix(arg, "-") {
			for _, c := range arg[1:] {
				switch c {
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
					// Ignored
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
					fmt.Fprintf(os.Stderr, "cat: invalid option -- '%c'\n", c)
					printTryHelp()
					os.Exit(1)
				}
			}
			continue
		}
		files = append(files, arg)
	}

	if help {
		printHelp()
		os.Exit(0)
	}
	if version {
		printVersion()
		os.Exit(0)
	}

	if len(files) == 0 {
		files = append(files, "-")
	}

	ostat, err := os.Stdout.Stat()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cat: standard output: %v\n", err)
		os.Exit(1)
	}

	var ok = true
	state := &catState{
		newlines:  0,
		pendingCR: false,
		lineCount: 0,
	}

	w := bufio.NewWriter(os.Stdout)

	for _, filename := range files {
		var file *os.File
		if filename == "-" {
			file = os.Stdin
		} else {
			var err error
			file, err = os.Open(filename)
			if err != nil {
				fmt.Fprintf(os.Stderr, "cat: %s: %v\n", filename, err)
				ok = false
				continue
			}
		}

		istat, err := file.Stat()
		if err != nil {
			fmt.Fprintf(os.Stderr, "cat: %s: %v\n", filename, err)
			if filename != "-" {
				file.Close()
			}
			ok = false
			continue
		}

		if filename != "-" && os.SameFile(istat, ostat) {
			if istat.Mode().IsRegular() && ostat.Mode().IsRegular() {
				fmt.Fprintf(os.Stderr, "cat: %s: input file is output file\n", filename)
				file.Close()
				ok = false
				continue
			}
		}

		if !(number || showEnds || showNonprinting || showTabs || squeezeBlank) {
			if ferr := w.Flush(); ferr != nil {
				fmt.Fprintf(os.Stderr, "cat: write error: %v\n", ferr)
				os.Exit(1)
			}
			if !simpleCat(file, os.Stdout, filename) {
				ok = false
			}
		} else {
			flusher := flushingReader{r: file, w: w}
			r := bufio.NewReader(flusher)
			err := state.cat(r, w, showNonprinting, showTabs, number, numberNonblank, showEnds, squeezeBlank)
			if ferr := w.Flush(); ferr != nil {
				fmt.Fprintf(os.Stderr, "cat: write error: %v\n", ferr)
				os.Exit(1)
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "cat: %s: %v\n", filename, err)
				ok = false
			}
		}

		if filename != "-" {
			file.Close()
		}
	}

	if state.pendingCR {
		if _, err := os.Stdout.Write([]byte{'\r'}); err != nil {
			fmt.Fprintf(os.Stderr, "cat: write error: %v\n", err)
			os.Exit(1)
		}
	}

	if !ok {
		os.Exit(1)
	}
}
