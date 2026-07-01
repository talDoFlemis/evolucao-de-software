package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
)

type options struct {
	showAll        bool
	numberNonBlank bool
	showEnds       bool
	number         bool
	squeezeBlank   bool
	showNonPrint   bool
	showTabs       bool
}

type state struct {
	lineNumber   int64
	atLineStart  bool
	prevBlank    bool
	pendingCR    bool
}

func main() {
	opts, files, err := parseArgs(os.Args[1:])
	if err != nil {
		usageErr(err)
		os.Exit(1)
	}

	if opts.showAll {
		opts.showNonPrint = true
		opts.showEnds = true
		opts.showTabs = true
	}
	if opts.numberNonBlank {
		opts.number = true
	}

	stdout := bufio.NewWriter(os.Stdout)
	defer func() {
		_ = stdout.Flush()
	}()

	st := &state{lineNumber: 1, atLineStart: true}
	ok := true
	writeFailed := false
	for _, name := range files {
		if err := processOne(name, opts, st, stdout); err != nil {
			ok = false
			if err == errWriteFailed {
				fmt.Fprintln(os.Stderr, "cat: write error")
				writeFailed = true
				break
			}
		}
	}

	if !writeFailed && st.pendingCR {
		if err := stdout.WriteByte('\r'); err != nil {
			fmt.Fprintln(os.Stderr, "cat: write error")
			os.Exit(1)
		}
		st.pendingCR = false
	}

	if !writeFailed {
		if err := stdout.Flush(); err != nil {
		fmt.Fprintln(os.Stderr, "cat: write error")
		ok = false
		}
	}

	if !ok {
		os.Exit(1)
	}
}

var (
	errWriteFailed = errors.New("write failed")
	errInputFailed = errors.New("input failed")
)

func parseArgs(args []string) (options, []string, error) {
	opts := options{}
	files := make([]string, 0, len(args))
	endOfOpts := false

	for _, arg := range args {
		if endOfOpts {
			files = append(files, arg)
			continue
		}
		if arg == "--" {
			endOfOpts = true
			continue
		}
		if len(arg) < 2 || arg[0] != '-' || arg == "-" {
			files = append(files, arg)
			continue
		}

		if len(arg) >= 3 && arg[:2] == "--" {
			switch arg {
			case "--show-all":
				opts.showAll = true
			case "--number-nonblank":
				opts.numberNonBlank = true
			case "--show-ends":
				opts.showEnds = true
			case "--number":
				opts.number = true
			case "--squeeze-blank":
				opts.squeezeBlank = true
			case "--show-nonprinting":
				opts.showNonPrint = true
			case "--show-tabs":
				opts.showTabs = true
			default:
				return options{}, nil, fmt.Errorf("unknown option %q", arg)
			}
			continue
		}

		for i := 1; i < len(arg); i++ {
			switch arg[i] {
			case 'A':
				opts.showAll = true
			case 'b':
				opts.numberNonBlank = true
			case 'e':
				opts.showEnds = true
				opts.showNonPrint = true
			case 'E':
				opts.showEnds = true
			case 'n':
				opts.number = true
			case 's':
				opts.squeezeBlank = true
			case 't':
				opts.showTabs = true
				opts.showNonPrint = true
			case 'u':
			case 'v':
				opts.showNonPrint = true
			case 'T':
				opts.showTabs = true
			default:
				return options{}, nil, fmt.Errorf("unknown option -%c", arg[i])
			}
		}
	}

	if len(files) == 0 {
		files = []string{"-"}
	}
	return opts, files, nil
}

func usageErr(err error) {
	fmt.Fprintf(os.Stderr, "cat: %v\n", err)
	fmt.Fprintln(os.Stderr, "Usage: cat [OPTION]... [FILE]...")
}

func processOne(name string, opts options, st *state, out *bufio.Writer) error {
	if !formatted(opts) {
		return copyPlain(name, out)
	}

	var r io.ReadCloser
	if name == "-" {
		r = io.NopCloser(os.Stdin)
	} else {
		f, err := os.Open(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, err)
			return errInputFailed
		}
		r = f
	}
	defer r.Close()

	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if err := transform(buf[:n], opts, st, out); err != nil {
				if err == errWriteFailed {
					return err
				}
				fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, err)
				return errInputFailed
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, err)
			return errInputFailed
		}
	}
}

func copyPlain(name string, out *bufio.Writer) error {
	var r io.ReadCloser
	if name == "-" {
		r = io.NopCloser(os.Stdin)
	} else {
		f, err := os.Open(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, err)
			return errInputFailed
		}
		r = f
	}
	defer r.Close()

	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return errWriteFailed
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, err)
			return errInputFailed
		}
	}
}

func formatted(opts options) bool {
	return opts.number || opts.numberNonBlank || opts.squeezeBlank || opts.showNonPrint || opts.showEnds || opts.showTabs
}

func transform(chunk []byte, opts options, st *state, out *bufio.Writer) error {
	for _, b := range chunk {
		if st.pendingCR && b != '\n' {
			if err := out.WriteByte('\r'); err != nil {
				return errWriteFailed
			}
			st.pendingCR = false
		}

		if b == '\n' {
			if st.pendingCR {
				if err := out.WriteByte('^'); err != nil {
					return errWriteFailed
				}
				if err := out.WriteByte('M'); err != nil {
					return errWriteFailed
				}
				st.pendingCR = false
			}

			blank := st.atLineStart
			if blank && opts.squeezeBlank && st.prevBlank {
				st.atLineStart = true
				st.prevBlank = true
				continue
			}
			if blank && opts.number && !opts.numberNonBlank {
				if err := writeLineNumber(out, st.lineNumber); err != nil {
					return err
				}
				st.lineNumber++
			}
			if opts.showEnds {
				if err := out.WriteByte('$'); err != nil {
					return errWriteFailed
				}
			}
			if err := out.WriteByte('\n'); err != nil {
				return errWriteFailed
			}
			st.atLineStart = true
			st.prevBlank = blank
			continue
		}

		if st.atLineStart && opts.number {
			if err := writeLineNumber(out, st.lineNumber); err != nil {
				return err
			}
			st.lineNumber++
			st.atLineStart = false
		}

		if opts.showNonPrint {
			if b == '\t' {
				if opts.showTabs {
					if err := out.WriteByte('^'); err != nil {
						return errWriteFailed
					}
					if err := out.WriteByte('I'); err != nil {
						return errWriteFailed
					}
				} else {
					if err := out.WriteByte('\t'); err != nil {
						return errWriteFailed
					}
				}
				st.atLineStart = false
				continue
			}
			if b < 32 {
				if err := out.WriteByte('^'); err != nil {
					return errWriteFailed
				}
				if err := out.WriteByte(b + 64); err != nil {
					return errWriteFailed
				}
				st.atLineStart = false
				continue
			}
			if b >= 32 {
				switch {
				case b < 127:
					if err := out.WriteByte(b); err != nil {
						return errWriteFailed
					}
				case b == 127:
					if err := out.WriteByte('^'); err != nil {
						return errWriteFailed
					}
					if err := out.WriteByte('?'); err != nil {
						return errWriteFailed
					}
				default:
					if err := out.WriteByte('M'); err != nil {
						return errWriteFailed
					}
					if err := out.WriteByte('-'); err != nil {
						return errWriteFailed
					}
					if b >= 128+32 {
						if b < 128+127 {
							if err := out.WriteByte(b - 128); err != nil {
								return errWriteFailed
							}
						} else {
							if err := out.WriteByte('^'); err != nil {
								return errWriteFailed
							}
							if err := out.WriteByte('?'); err != nil {
								return errWriteFailed
							}
						}
					} else {
						if err := out.WriteByte('^'); err != nil {
							return errWriteFailed
						}
						if err := out.WriteByte(b - 128 + 64); err != nil {
							return errWriteFailed
						}
					}
				}
				st.atLineStart = false
				continue
			}
			if err := out.WriteByte(b); err != nil {
				return errWriteFailed
			}
			st.atLineStart = false
			continue
		}

		if b == '\t' && opts.showTabs {
			if err := out.WriteByte('^'); err != nil {
				return errWriteFailed
			}
			if err := out.WriteByte('I'); err != nil {
				return errWriteFailed
			}
			st.atLineStart = false
			continue
		}
		if b == '\r' && opts.showEnds {
			st.pendingCR = true
			st.atLineStart = false
			continue
		}
		if err := out.WriteByte(b); err != nil {
			return errWriteFailed
		}
		st.atLineStart = false
	}
	return nil
}

func writeLineNumber(out *bufio.Writer, n int64) error {
	str := strconv.FormatInt(n, 10)
	if len(str) < 6 {
		for i := 0; i < 6-len(str); i++ {
			if err := out.WriteByte(' '); err != nil {
				return errWriteFailed
			}
		}
	}
	if _, err := out.WriteString(str); err != nil {
		return errWriteFailed
	}
	if err := out.WriteByte('\t'); err != nil {
		return errWriteFailed
	}
	return nil
}
