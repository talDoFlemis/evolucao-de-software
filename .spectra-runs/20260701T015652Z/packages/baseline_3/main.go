package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type options struct {
	numberAll       bool
	numberNonblank  bool
	squeezeBlank    bool
	showEnds        bool
	showNonprinting bool
	showTabs        bool
}

func main() {
	opts, files, showHelp, showVersion, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "cat:", err)
		os.Exit(1)
	}

	if showHelp {
		printHelp()
		return
	}
	if showVersion {
		fmt.Println("cat (Go baseline)")
		return
	}

	if len(files) == 0 {
		files = []string{"-"}
	}

	stdoutInfo, err := os.Stdout.Stat()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cat:", err)
		os.Exit(1)
	}

	stdoutFileInfo := stdoutInfo
	writer := bufio.NewWriterSize(os.Stdout, 64*1024)
	defer func() {
		if flushErr := writer.Flush(); flushErr != nil {
			fmt.Fprintln(os.Stderr, "cat:", flushErr)
			os.Exit(1)
		}
	}()

	ok := true
	for _, name := range files {
		if name == "-" {
			if err := processReader("-", os.Stdin, writer, opts, nil); err != nil {
				fmt.Fprintln(os.Stderr, err)
				ok = false
			}
			continue
		}

		f, err := os.Open(name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, err)
			ok = false
			continue
		}

		info, statErr := f.Stat()
		if statErr != nil {
			fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, statErr)
			_ = f.Close()
			ok = false
			continue
		}

		if sameRegularFile(info, stdoutFileInfo) {
			fmt.Fprintf(os.Stderr, "cat: %s: input file is output file\n", name)
			_ = f.Close()
			ok = false
			continue
		}

		if err := processReader(name, f, writer, opts, info); err != nil {
			fmt.Fprintln(os.Stderr, err)
			ok = false
		}
		if closeErr := f.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, closeErr)
			ok = false
		}
	}

	if !ok {
		os.Exit(1)
	}
}

func parseArgs(args []string) (options, []string, bool, bool, error) {
	var opts options
	var files []string
	var help bool
	var version bool
	endOfOptions := false

	for _, arg := range args {
		if endOfOptions {
			files = append(files, arg)
			continue
		}
		if arg == "--" {
			endOfOptions = true
			continue
		}
		if arg == "--help" {
			help = true
			continue
		}
		if arg == "--version" {
			version = true
			continue
		}
		if strings.HasPrefix(arg, "--") {
			switch arg {
			case "--show-all":
				opts.showNonprinting = true
				opts.showEnds = true
				opts.showTabs = true
			case "--number-nonblank":
				opts.numberAll = true
				opts.numberNonblank = true
			case "--number":
				opts.numberAll = true
			case "--squeeze-blank":
				opts.squeezeBlank = true
			case "--show-nonprinting":
				opts.showNonprinting = true
			case "--show-ends":
				opts.showEnds = true
			case "--show-tabs":
				opts.showTabs = true
			case "--":
				endOfOptions = true
			default:
				return opts, nil, false, false, fmt.Errorf("unrecognized option %q", arg)
			}
			continue
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			for i := 1; i < len(arg); i++ {
				switch arg[i] {
				case 'A':
					opts.showNonprinting = true
					opts.showEnds = true
					opts.showTabs = true
				case 'b':
					opts.numberAll = true
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
					// Ignored, matching GNU cat.
				case 'v':
					opts.showNonprinting = true
				default:
					return opts, nil, false, false, fmt.Errorf("unrecognized option %q", "-"+string(arg[i]))
				}
			}
			continue
		}
		files = append(files, arg)
	}

	return opts, files, help, version, nil
}

func processReader(name string, r io.Reader, w *bufio.Writer, opts options, info os.FileInfo) error {
	_ = name
	_ = info

	buf := make([]byte, 32*1024)
	lineStart := true
	blankRun := 0
	lineNum := 0
	pendingCR := false

	for {
		n, err := r.Read(buf)
		if n > 0 {
			for i := 0; i < n; i++ {
				b := buf[i]

				if pendingCR {
					pendingCR = false
					if b == '\n' {
						if err := writeVisibleCRBeforeLF(w); err != nil {
							return err
						}
					} else {
						if err := w.WriteByte('\r'); err != nil {
							return err
						}
					}
				}

				if opts.squeezeBlank && lineStart && b == '\n' {
					if blankRun > 0 {
						continue
					}
				}

				if lineStart {
					if b == '\n' {
						if opts.numberAll && !opts.numberNonblank {
							lineNum++
							if err := writeLineNumber(w, lineNum); err != nil {
								return err
							}
						}
						if err := writeNewline(w, opts, false); err != nil {
							return err
						}
						blankRun = 1
						continue
					}

					if opts.numberAll || opts.numberNonblank {
						lineNum++
						if err := writeLineNumber(w, lineNum); err != nil {
							return err
						}
					}
					lineStart = false
					blankRun = 0
				}

				if opts.showNonprinting {
					if err := writeQuotedByte(w, b, opts.showTabs, opts.showEnds); err != nil {
						return err
					}
				} else {
					switch b {
					case '\t':
						if opts.showTabs {
							if err := w.WriteByte('^'); err != nil {
								return err
							}
							if err := w.WriteByte('I'); err != nil {
								return err
							}
						} else if err := w.WriteByte('\t'); err != nil {
							return err
						}
					case '\n':
						if err := writeNewline(w, opts, false); err != nil {
							return err
						}
						lineStart = true
						blankRun = 0
					case '\r':
						if opts.showEnds {
							if i+1 < n {
								if buf[i+1] == '\n' {
									if err := writeVisibleCRBeforeLF(w); err != nil {
										return err
									}
								} else if err := w.WriteByte('\r'); err != nil {
									return err
								}
							} else {
								pendingCR = true
								continue
							}
						} else if err := w.WriteByte('\r'); err != nil {
							return err
						}
					default:
						if err := w.WriteByte(b); err != nil {
							return err
						}
					}
				}

				if !opts.showNonprinting && b != '\n' {
					lineStart = false
				}
				if opts.showNonprinting && b != '\n' {
					lineStart = false
				}
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("cat: %s: %w", name, err)
		}
	}

	if pendingCR {
		if err := w.WriteByte('\r'); err != nil {
			return err
		}
	}

	return nil
}

func writeVisibleCRBeforeLF(w *bufio.Writer) error {
	if err := w.WriteByte('^'); err != nil {
		return err
	}
	if err := w.WriteByte('M'); err != nil {
		return err
	}
	return nil
}

func writeNewline(w *bufio.Writer, opts options, fromQuoted bool) error {
	_ = fromQuoted
	if opts.showEnds {
		if err := w.WriteByte('$'); err != nil {
			return err
		}
	}
	return w.WriteByte('\n')
}

func writeLineNumber(w *bufio.Writer, n int) error {
	_, err := fmt.Fprintf(w, "%6d\t", n)
	return err
}

func writeQuotedByte(w *bufio.Writer, b byte, showTabs, showEnds bool) error {
	switch {
	case b == '\n':
		if showEnds {
			if err := w.WriteByte('$'); err != nil {
				return err
			}
		}
		return w.WriteByte('\n')
	case b == '\t' && !showTabs:
		return w.WriteByte('\t')
	case b >= 32 && b < 127:
		return w.WriteByte(b)
	case b == 127:
		if err := w.WriteByte('^'); err != nil {
			return err
		}
		return w.WriteByte('?')
	case b >= 128:
		if err := w.WriteByte('M'); err != nil {
			return err
		}
		if err := w.WriteByte('-'); err != nil {
			return err
		}
		c := b - 128
		if c >= 32 && c < 127 {
			return w.WriteByte(c)
		}
		if c == 127 {
			if err := w.WriteByte('^'); err != nil {
				return err
			}
			return w.WriteByte('?')
		}
		if err := w.WriteByte('^'); err != nil {
			return err
		}
		return w.WriteByte(c + 64)
	default:
		if err := w.WriteByte('^'); err != nil {
			return err
		}
		return w.WriteByte(b + 64)
	}
}

func sameRegularFile(a, b os.FileInfo) bool {
	if a == nil || b == nil {
		return false
	}
	if !a.Mode().IsRegular() || !b.Mode().IsRegular() {
		return false
	}
	as, ok := a.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	bs, ok := b.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return as.Dev == bs.Dev && as.Ino == bs.Ino
}

func printHelp() {
	name := filepath.Base(os.Args[0])
	fmt.Printf("Usage: %s [OPTION]... [FILE]...\n", name)
	fmt.Println("Concatenate FILE(s) to standard output.")
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
}
