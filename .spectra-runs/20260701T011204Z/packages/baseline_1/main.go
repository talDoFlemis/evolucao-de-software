package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

type options struct {
	numberAll       bool
	numberNonBlank  bool
	squeezeBlank    bool
	showEnds        bool
	showNonPrinting bool
	showTabs        bool
}

type state struct {
	lineNum      int
	atLineStart  bool
	lineHasText  bool
	prevLineBlank bool
	pendingCR    bool
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	opts, files, help, version, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cat:", err)
		return 1
	}
	if help {
		usage(os.Stdout)
		return 0
	}
	if version {
		fmt.Fprintln(os.Stdout, "cat (Go baseline)")
		return 0
	}

	opts.numberAll = opts.numberAll && !opts.numberNonBlank

	outInfo, err := os.Stdout.Stat()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cat:", err)
		return 1
	}

	writer := bufio.NewWriterSize(os.Stdout, 32*1024)
	defer writer.Flush()

	st := &state{lineNum: 0, atLineStart: true}
	ok := true
	if len(files) == 0 {
		files = []string{"-"}
	}
	for _, name := range files {
		if name != "-" {
			inInfo, statErr := os.Stat(name)
			if statErr == nil && outInfo.Mode().IsRegular() && inInfo.Mode().IsRegular() && os.SameFile(inInfo, outInfo) {
				fmt.Fprintf(os.Stderr, "cat: %s: input file is output file\n", name)
				ok = false
				continue
			}
		}

		var in *os.File
		if name == "-" {
			in = os.Stdin
		} else {
			in, err = os.Open(name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, err)
				ok = false
				continue
			}
		}

		if err := process(in, writer, st, opts); err != nil {
			fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, err)
			ok = false
		}
		if name != "-" {
			if closeErr := in.Close(); closeErr != nil {
				fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, closeErr)
				ok = false
			}
		}
	}

	if err := writer.Flush(); err != nil {
		fmt.Fprintln(os.Stderr, "cat:", err)
		return 1
	}
	if ok {
		return 0
	}
	return 1
}

func parseArgs(args []string) (options, []string, bool, bool, error) {
	var opts options
	var files []string
	var help bool
	var version bool

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			files = append(files, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "--") {
			switch arg {
			case "--show-all":
				opts.showNonPrinting = true
				opts.showEnds = true
				opts.showTabs = true
			case "--number-nonblank":
				opts.numberNonBlank = true
			case "--show-ends":
				opts.showEnds = true
			case "--number":
				opts.numberAll = true
			case "--squeeze-blank":
				opts.squeezeBlank = true
			case "--show-tabs":
				opts.showTabs = true
			case "--show-nonprinting":
				opts.showNonPrinting = true
			case "--help":
				help = true
			case "--version":
				version = true
			default:
				return opts, nil, false, false, fmt.Errorf("unrecognized option %q", arg)
			}
			continue
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			for j := 1; j < len(arg); j++ {
				switch arg[j] {
				case 'A':
					opts.showNonPrinting = true
					opts.showEnds = true
					opts.showTabs = true
				case 'b':
					opts.numberNonBlank = true
				case 'e':
					opts.showEnds = true
					opts.showNonPrinting = true
				case 'E':
					opts.showEnds = true
				case 'n':
					opts.numberAll = true
				case 's':
					opts.squeezeBlank = true
				case 't':
					opts.showTabs = true
					opts.showNonPrinting = true
				case 'T':
					opts.showTabs = true
				case 'u':
					// Ignored.
				case 'v':
					opts.showNonPrinting = true
				default:
					return opts, nil, false, false, fmt.Errorf("unrecognized option %q", "-"+string(arg[j]))
				}
			}
			continue
		}
		files = append(files, arg)
	}

	return opts, files, help, version, nil
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "Usage: cat [OPTION]... [FILE]...")
	fmt.Fprintln(w, "Concatenate FILE(s) to standard output.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "  -A, --show-all           equivalent to -vET")
	fmt.Fprintln(w, "  -b, --number-nonblank    number nonempty output lines, overrides -n")
	fmt.Fprintln(w, "  -e                       equivalent to -vE")
	fmt.Fprintln(w, "  -E, --show-ends          display $ at end of each line")
	fmt.Fprintln(w, "  -n, --number             number all output lines")
	fmt.Fprintln(w, "  -s, --squeeze-blank      suppress repeated empty output lines")
	fmt.Fprintln(w, "  -t                       equivalent to -vT")
	fmt.Fprintln(w, "  -T, --show-tabs          display TAB characters as ^I")
	fmt.Fprintln(w, "  -u                       (ignored)")
	fmt.Fprintln(w, "  -v, --show-nonprinting   use ^ and M- notation, except for LFD and TAB")
}

func process(r io.Reader, w *bufio.Writer, st *state, opts options) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if err := processBytes(buf[:n], w, st, opts); err != nil {
				return err
			}
		}
		if err != nil {
			if err == io.EOF {
				return flushPendingCR(w, st, opts, true)
			}
			return err
		}
	}
}

func processBytes(data []byte, w *bufio.Writer, st *state, opts options) error {
	for _, b := range data {
		if st.pendingCR {
			if b == '\n' {
				if st.atLineStart && (opts.numberAll || opts.numberNonBlank) {
					if err := writeLineNumber(w, st); err != nil {
						return err
					}
				}
				if err := writeCR(w); err != nil {
					return err
				}
				st.pendingCR = false
				st.lineHasText = true
				st.atLineStart = false
			} else {
				if st.atLineStart && (opts.numberAll || opts.numberNonBlank) {
					if err := writeLineNumber(w, st); err != nil {
						return err
					}
				}
				if err := writeRawByte(w, '\r'); err != nil {
					return err
				}
				st.pendingCR = false
				st.lineHasText = true
				st.atLineStart = false
			}
		}

		if b == '\n' {
			if !st.lineHasText {
				if opts.squeezeBlank && st.prevLineBlank {
					st.atLineStart = true
					continue
				}
				if opts.numberAll {
					if err := writeLineNumber(w, st); err != nil {
						return err
					}
				}
				st.prevLineBlank = true
			} else {
				st.prevLineBlank = false
			}
			if opts.showEnds {
				if err := writeRawByte(w, '$'); err != nil {
					return err
				}
			}
			if err := writeRawByte(w, '\n'); err != nil {
				return err
			}
			st.atLineStart = true
			st.lineHasText = false
			continue
		}

		if opts.showEnds && !opts.showNonPrinting && b == '\r' {
			st.pendingCR = true
			continue
		}

		if st.atLineStart && (opts.numberAll || opts.numberNonBlank) {
			if err := writeLineNumber(w, st); err != nil {
				return err
			}
		}
		st.atLineStart = false
		st.lineHasText = true
		st.prevLineBlank = false

		if err := writeDisplayByte(w, b, opts); err != nil {
			return err
		}
	}
	return nil
}

func flushPendingCR(w *bufio.Writer, st *state, opts options, eof bool) error {
	if !st.pendingCR {
		return nil
	}
	if st.atLineStart && (opts.numberAll || opts.numberNonBlank) {
		if err := writeLineNumber(w, st); err != nil {
			return err
		}
	}
	st.pendingCR = false
	st.atLineStart = false
	st.lineHasText = true
	st.prevLineBlank = false
	if eof {
		return writeRawByte(w, '\r')
	}
	return writeCR(w)
}

func writeLineNumber(w *bufio.Writer, st *state) error {
	st.lineNum++
	_, err := fmt.Fprintf(w, "%6d\t", st.lineNum)
	return err
}

func writeDisplayByte(w *bufio.Writer, b byte, opts options) error {
	if opts.showNonPrinting {
		switch {
		case b >= 32:
			if b < 127 {
				return writeRawByte(w, b)
			}
			if b == 127 {
				if err := writeRawByte(w, '^'); err != nil {
					return err
				}
				return writeRawByte(w, '?')
			}
			if err := writeRawByte(w, 'M'); err != nil {
				return err
			}
			if err := writeRawByte(w, '-'); err != nil {
				return err
			}
			if b >= 128+32 {
				if b < 128+127 {
					return writeRawByte(w, b-128)
				}
				if err := writeRawByte(w, '^'); err != nil {
					return err
				}
				return writeRawByte(w, '?')
			}
			if err := writeRawByte(w, '^'); err != nil {
				return err
			}
			return writeRawByte(w, b-128+64)
		case b == '\t' && !opts.showTabs:
			return writeRawByte(w, '\t')
		default:
			if err := writeRawByte(w, '^'); err != nil {
				return err
			}
			return writeRawByte(w, b+64)
		}
	}

	if b == '\t' && opts.showTabs {
		if err := writeRawByte(w, '^'); err != nil {
			return err
		}
		return writeRawByte(w, 'I')
	}
	return writeRawByte(w, b)
}

func writeRawByte(w *bufio.Writer, b byte) error {
	return w.WriteByte(b)
}

func writeCR(w *bufio.Writer) error {
	if err := writeRawByte(w, '^'); err != nil {
		return err
	}
	return writeRawByte(w, 'M')
}
