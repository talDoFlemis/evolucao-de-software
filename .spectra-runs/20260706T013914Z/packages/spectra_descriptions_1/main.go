package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
)

type options struct {
	number          bool
	numberNonblank  bool
	squeezeBlank    bool
	showEnds        bool
	showTabs        bool
	showNonprinting bool
}

type state struct {
	lineNumber     int
	blankRun       int
	atLineStart    bool
	lineHasContent bool
	pendingCR      bool
}

func main() {
	opts, files, ok := parseArgs(os.Args[1:])
	if !ok {
		os.Exit(1)
	}
	if len(files) == 0 {
		files = []string{"-"}
	}

	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()

	status := 0
	if !needsFormatting(opts) {
		buf := make([]byte, 32*1024)
		for _, name := range files {
			r, closeFn, err := openInput(name)
			if err != nil {
				reportFileError(name, err)
				status = 1
				continue
			}
			if _, err := io.CopyBuffer(w, r, buf); err != nil {
				reportWriteError(err)
				closeFn()
				os.Exit(1)
			}
			if err := closeFn(); err != nil {
				reportFileError(name, err)
				status = 1
			}
		}
		if err := w.Flush(); err != nil {
			reportWriteError(err)
			os.Exit(1)
		}
		os.Exit(status)
	}

	st := state{atLineStart: true}
	buf := make([]byte, 32*1024)
	for _, name := range files {
		r, closeFn, err := openInput(name)
		if err != nil {
			reportFileError(name, err)
			status = 1
			continue
		}
		if err := processInput(r, w, opts, &st, buf); err != nil {
			if err == io.EOF {
				err = nil
			}
			if err != nil {
				reportFileError(name, err)
				status = 1
			}
		}
		if err := closeFn(); err != nil {
			reportFileError(name, err)
			status = 1
		}
	}

	if st.pendingCR {
		if err := w.WriteByte('\r'); err != nil {
			reportWriteError(err)
			os.Exit(1)
		}
	}
	if err := w.Flush(); err != nil {
		reportWriteError(err)
		os.Exit(1)
	}
	os.Exit(status)
}

func parseArgs(args []string) (options, []string, bool) {
	var opts options
	var files []string
	endOfOptions := false

	for _, arg := range args {
		if endOfOptions || arg == "-" || len(arg) == 0 || arg[0] != '-' {
			files = append(files, arg)
			continue
		}
		if arg == "--" {
			endOfOptions = true
			continue
		}
		if len(arg) > 1 && arg[1] == '-' {
			if !applyLongOption(arg[2:], &opts) {
				fmt.Fprintf(os.Stderr, "cat: unrecognized option '%s'\n", arg)
				return options{}, nil, false
			}
			continue
		}
		for i := 1; i < len(arg); i++ {
			switch arg[i] {
			case 'A':
				opts.showNonprinting = true
				opts.showEnds = true
				opts.showTabs = true
			case 'b':
				opts.number = true
				opts.numberNonblank = true
			case 'e':
				opts.showNonprinting = true
				opts.showEnds = true
			case 'E':
				opts.showEnds = true
			case 'n':
				opts.number = true
			case 's':
				opts.squeezeBlank = true
			case 't':
				opts.showNonprinting = true
				opts.showTabs = true
			case 'T':
				opts.showTabs = true
			case 'u':
			case 'v':
				opts.showNonprinting = true
			default:
				fmt.Fprintf(os.Stderr, "cat: invalid option -- '%c'\n", arg[i])
				return options{}, nil, false
			}
		}
	}

	return opts, files, true
}

func applyLongOption(name string, opts *options) bool {
	switch name {
	case "number-nonblank":
		opts.number = true
		opts.numberNonblank = true
	case "number":
		opts.number = true
	case "squeeze-blank":
		opts.squeezeBlank = true
	case "show-nonprinting":
		opts.showNonprinting = true
	case "show-ends":
		opts.showEnds = true
	case "show-tabs":
		opts.showTabs = true
	case "show-all":
		opts.showNonprinting = true
		opts.showEnds = true
		opts.showTabs = true
	default:
		return false
	}
	return true
}

func needsFormatting(opts options) bool {
	return opts.number || opts.showEnds || opts.showNonprinting || opts.showTabs || opts.squeezeBlank
}

func openInput(name string) (io.Reader, func() error, error) {
	if name == "-" {
		return os.Stdin, func() error { return nil }, nil
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, nil, err
	}
	return f, f.Close, nil
}

func processInput(r io.Reader, w *bufio.Writer, opts options, st *state, buf []byte) error {
	for {
		n, err := r.Read(buf)
		if n > 0 {
			for _, b := range buf[:n] {
				if opts.showEnds && !opts.showNonprinting && b == '\r' {
					if st.pendingCR {
						if err := w.WriteByte('\r'); err != nil {
							return err
						}
						st.pendingCR = false
					}
					if st.atLineStart && opts.number {
						st.lineNumber++
						if err := writeLineNumber(w, st.lineNumber); err != nil {
							return err
						}
					}
					st.atLineStart = false
					st.lineHasContent = true
					st.pendingCR = true
					continue
				}

				if b != '\n' && st.pendingCR {
					if err := w.WriteByte('\r'); err != nil {
						return err
					}
					st.pendingCR = false
				}

				if b == '\n' {
					isBlank := !st.lineHasContent
					if isBlank {
						st.blankRun++
					} else {
						st.blankRun = 0
					}
					if !(isBlank && opts.squeezeBlank && st.blankRun > 1) {
						if isBlank && opts.number && !opts.numberNonblank {
							st.lineNumber++
							if err := writeLineNumber(w, st.lineNumber); err != nil {
								return err
							}
						}
						if opts.showEnds {
							if st.pendingCR {
								if _, err := w.WriteString("^M"); err != nil {
									return err
								}
								st.pendingCR = false
							}
							if err := w.WriteByte('$'); err != nil {
								return err
							}
						}
						if err := w.WriteByte('\n'); err != nil {
							return err
						}
					}
					st.atLineStart = true
					st.lineHasContent = false
					continue
				}

				if st.atLineStart && opts.number {
					st.lineNumber++
					if err := writeLineNumber(w, st.lineNumber); err != nil {
						return err
					}
				}
				st.atLineStart = false
				st.lineHasContent = true

				if opts.showNonprinting {
					if err := writeVisibleByte(w, b, opts.showTabs); err != nil {
						return err
					}
					continue
				}
				if b == '\t' && opts.showTabs {
					if _, err := w.WriteString("^I"); err != nil {
						return err
					}
					continue
				}
				if err := w.WriteByte(b); err != nil {
					return err
				}
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func writeLineNumber(w *bufio.Writer, n int) error {
	_, err := fmt.Fprintf(w, "%6d\t", n)
	return err
}

func writeVisibleByte(w *bufio.Writer, b byte, showTabs bool) error {
	if b >= 32 {
		if b < 127 {
			return w.WriteByte(b)
		}
		if b == 127 {
			_, err := w.WriteString("^?")
			return err
		}
		if _, err := w.WriteString("M-"); err != nil {
			return err
		}
		if b >= 160 {
			if b < 255 {
				return w.WriteByte(b - 128)
			}
			_, err := w.WriteString("^?")
			return err
		}
		if err := w.WriteByte('^'); err != nil {
			return err
		}
		return w.WriteByte(b - 128 + 64)
	}
	if b == '\t' && !showTabs {
		return w.WriteByte('\t')
	}
	if err := w.WriteByte('^'); err != nil {
		return err
	}
	return w.WriteByte(b + 64)
}

func reportFileError(name string, err error) {
	fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, err)
}

func reportWriteError(err error) {
	fmt.Fprintf(os.Stderr, "cat: write error: %v\n", err)
}
