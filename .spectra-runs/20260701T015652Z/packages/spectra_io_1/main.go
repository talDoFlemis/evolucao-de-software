package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

type options struct {
	showAll         bool
	numberAll       bool
	numberNonblank  bool
	squeezeBlank    bool
	showEnds        bool
	showTabs        bool
	showNonprinting bool
	showHelp        bool
	showVersion     bool
}

func main() {
	args, opts, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if opts.showHelp {
		usage(os.Stdout)
		return
	}
	if opts.showVersion {
		fmt.Println("cat (Go translation) 1.0")
		return
	}

	if len(args) == 0 {
		args = []string{"-"}
	}

	state := &catState{nextLine: 1, atLineStart: true}
	var hadErr bool
	for _, name := range args {
		if name == "-" {
			if err := copyStream(os.Stdin, os.Stdout, state, opts); err != nil {
				reportFileError(name, err)
				hadErr = true
			}
			continue
		}
		f, err := os.Open(name)
		if err != nil {
			reportFileError(name, err)
			hadErr = true
			continue
		}
		if err := copyStream(f, os.Stdout, state, opts); err != nil {
			reportFileError(name, err)
			hadErr = true
		}
		_ = f.Close()
	}

	if hadErr {
		os.Exit(1)
	}
}

type catState struct {
	nextLine            int64
	consecutiveNewlines int
	pendingCR           bool
	atLineStart         bool
}

func parseArgs(args []string) ([]string, options, error) {
	var opts options
	var files []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			files = append(files, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "--") && len(a) > 2 {
			switch a {
			case "--help":
				opts.showHelp = true
			case "--version":
				opts.showVersion = true
			case "--number":
				opts.numberAll = true
			case "--number-nonblank":
				opts.numberNonblank = true
			case "--squeeze-blank":
				opts.squeezeBlank = true
			case "--show-ends":
				opts.showEnds = true
			case "--show-tabs":
				opts.showTabs = true
			case "--show-nonprinting":
				opts.showNonprinting = true
			case "--show-all":
				opts.showAll = true
			default:
				return nil, options{}, fmt.Errorf("%s: unrecognized option %q", progName(), a)
			}
			continue
		}
		if len(a) > 1 && a[0] == '-' {
			for j := 1; j < len(a); j++ {
				switch a[j] {
				case 'A':
					opts.showAll = true
				case 'b':
					opts.numberNonblank = true
				case 'e':
					opts.showEnds = true
					opts.showNonprinting = true
				case 'E':
					opts.showEnds = true
				case 'n':
					opts.numberAll = true
				case 's':
					opts.squeezeBlank = true
				case 't':
					opts.showTabs = true
					opts.showNonprinting = true
				case 'T':
					opts.showTabs = true
				case 'u':
					// Ignored, GNU cat is unbuffered by default here.
				case 'v':
					opts.showNonprinting = true
				default:
					return nil, options{}, fmt.Errorf("%s: invalid option -- %c", progName(), a[j])
				}
			}
			continue
		}
		files = append(files, a)
	}

	if opts.showAll {
		opts.showNonprinting = true
		opts.showEnds = true
		opts.showTabs = true
	}
	if opts.numberNonblank {
		opts.numberAll = false
	}
	if opts.showHelp || opts.showVersion {
		return nil, opts, nil
	}
	return files, opts, nil
}

func usage(w io.Writer) {
	fmt.Fprintf(w, "Usage: %s [OPTION]... [FILE]...\n", progName())
	fmt.Fprintln(w, "Concatenate FILE(s) to standard output.")
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
	fmt.Fprintln(w, "      --help               display this help and exit")
	fmt.Fprintln(w, "      --version            output version information and exit")
}

func progName() string {
	if len(os.Args) == 0 {
		return "cat"
	}
	return filepath.Base(os.Args[0])
}

func reportFileError(name string, err error) {
	msg := err.Error()
	var pe *os.PathError
	if errors.As(err, &pe) {
		msg = pe.Err.Error()
	}
	if len(msg) > 0 {
		r := []rune(msg)
		r[0] = unicode.ToUpper(r[0])
		msg = string(r)
	}
	fmt.Fprintf(os.Stderr, "%s: %s: %s\n", progName(), name, msg)
}

func copyStream(r io.Reader, w io.Writer, state *catState, opts options) error {
	buf := make([]byte, 32*1024)
	lineStart := state.atLineStart
	defer func() { state.atLineStart = lineStart }()
	for {
		n, err := r.Read(buf)
		if n > 0 {
			for i := 0; i < n; i++ {
				b := buf[i]

				if state.pendingCR && b != '\n' {
					if err := writeByte(w, '\r'); err != nil {
						return err
					}
					state.pendingCR = false
				}

				if !opts.showNonprinting && opts.showEnds && b == '\r' {
					state.pendingCR = true
					continue
				}

				if b == '\n' {
					if opts.squeezeBlank && state.consecutiveNewlines >= 2 {
						state.pendingCR = false
						state.consecutiveNewlines++
						lineStart = true
						continue
					}
					if state.pendingCR {
						if err := writeString(w, "^M"); err != nil {
							return err
						}
						state.pendingCR = false
					}
					if opts.numberAll && !opts.numberNonblank && lineStart {
						if err := writeString(w, numberedLine(state.nextLine)); err != nil {
							return err
						}
						state.nextLine++
					}
					if opts.showEnds {
						if err := writeByte(w, '$'); err != nil {
							return err
						}
					}
					if err := writeByte(w, '\n'); err != nil {
						return err
					}
					state.consecutiveNewlines++
					lineStart = true
					continue
				}

				if lineStart {
					if opts.numberAll || opts.numberNonblank {
						if err := writeString(w, numberedLine(state.nextLine)); err != nil {
							return err
						}
						state.nextLine++
					}
					lineStart = false
				}

				state.consecutiveNewlines = 0
				if err := writeTransformedByte(w, b, opts); err != nil {
					return err
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				if state.pendingCR {
					if err := writeByte(w, '\r'); err != nil {
						return err
					}
					state.pendingCR = false
				}
				return nil
			}
			return err
		}
	}
}

func numberedLine(n int64) string {
	return fmt.Sprintf("%6d\t", n)
}

func writeTransformedByte(w io.Writer, b byte, opts options) error {
	if opts.showNonprinting {
		if b >= 32 {
			if b < 127 {
				return writeByte(w, b)
			}
			if b == 127 {
				if err := writeByte(w, '^'); err != nil {
					return err
				}
				return writeByte(w, '?')
			}
			if err := writeString(w, "M-"); err != nil {
				return err
			}
			b -= 128
			if b >= 32 {
				if b < 127 {
					return writeByte(w, b)
				}
				if err := writeByte(w, '^'); err != nil {
					return err
				}
				return writeByte(w, '?')
			}
			if err := writeByte(w, '^'); err != nil {
				return err
			}
			return writeByte(w, b+64)
		}
		if b == '\t' && !opts.showTabs {
			return writeByte(w, '\t')
		}
		if err := writeByte(w, '^'); err != nil {
			return err
		}
		return writeByte(w, b+64)
	}

	if b == '\t' && opts.showTabs {
		if err := writeByte(w, '^'); err != nil {
			return err
		}
		return writeByte(w, 'I')
	}
	return writeByte(w, b)
}

func writeByte(w io.Writer, b byte) error {
	var buf [1]byte
	buf[0] = b
	_, err := w.Write(buf[:])
	return err
}

func writeString(w io.Writer, s string) error {
	_, err := io.WriteString(w, s)
	return err
}
