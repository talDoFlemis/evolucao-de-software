package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

type options struct {
	number          bool
	numberNonblank  bool
	squeezeBlank    bool
	showEnds        bool
	showNonprinting bool
	showTabs        bool
	help            bool
	version         bool
}

type state struct {
	opts                options
	out                 *bufio.Writer
	lineNo              int
	atLineStart         bool
	consecutiveNewlines int
	pendingCR           bool
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	opts, files, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if opts.help {
		printUsage()
		return 0
	}
	if opts.version {
		fmt.Println("cat (Go baseline)")
		return 0
	}

	if len(files) == 0 {
		files = []string{"-"}
	}

	out := bufio.NewWriterSize(os.Stdout, 32*1024)
	s := &state{opts: opts, out: out, lineNo: 1, atLineStart: true}
	status := 0

	for _, name := range files {
		if err := s.process(name); err != nil {
			fmt.Fprintf(os.Stderr, "cat: %s: %v\n", name, err)
			status = 1
		}
	}

	if s.pendingCR {
		if err := s.writeString("\r"); err != nil {
			fmt.Fprintf(os.Stderr, "cat: %v\n", err)
			status = 1
		}
		s.pendingCR = false
	}

	if err := out.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "cat: %v\n", err)
		status = 1
	}

	return status
}

func parseArgs(args []string) (options, []string, error) {
	var opts options
	var files []string
	stop := false

	for _, arg := range args {
		if stop {
			files = append(files, arg)
			continue
		}
		if arg == "--" {
			stop = true
			continue
		}
		if arg == "-" {
			files = append(files, arg)
			continue
		}

		if strings.HasPrefix(arg, "--") {
			switch arg {
			case "--show-all":
				opts.showNonprinting = true
				opts.showEnds = true
				opts.showTabs = true
			case "--number-nonblank":
				opts.number = true
				opts.numberNonblank = true
			case "--show-ends":
				opts.showEnds = true
			case "--number":
				opts.number = true
			case "--squeeze-blank":
				opts.squeezeBlank = true
			case "--show-tabs":
				opts.showTabs = true
			case "--show-nonprinting":
				opts.showNonprinting = true
			case "--help":
				opts.help = true
			case "--version":
				opts.version = true
			default:
				return opts, nil, fmt.Errorf("cat: unrecognized option %s", arg)
			}
			continue
		}

		if strings.HasPrefix(arg, "-") {
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
					// Ignored.
				case 'v':
					opts.showNonprinting = true
				default:
					return opts, nil, fmt.Errorf("cat: invalid option -- %c", arg[i])
				}
			}
			continue
		}

		files = append(files, arg)
	}

	return opts, files, nil
}

func printUsage() {
	fmt.Fprintln(os.Stdout, "Usage: cat [OPTION]... [FILE]...")
	fmt.Fprintln(os.Stdout, "Concatenate FILE(s) to standard output.")
	fmt.Fprintln(os.Stdout, "  -A, --show-all           equivalent to -vET")
	fmt.Fprintln(os.Stdout, "  -b, --number-nonblank    number nonempty output lines, overrides -n")
	fmt.Fprintln(os.Stdout, "  -e                       equivalent to -vE")
	fmt.Fprintln(os.Stdout, "  -E, --show-ends          display $ at end of each line")
	fmt.Fprintln(os.Stdout, "  -n, --number             number all output lines")
	fmt.Fprintln(os.Stdout, "  -s, --squeeze-blank      suppress repeated empty output lines")
	fmt.Fprintln(os.Stdout, "  -t                       equivalent to -vT")
	fmt.Fprintln(os.Stdout, "  -T, --show-tabs          display TAB characters as ^I")
	fmt.Fprintln(os.Stdout, "  -u                       (ignored)")
	fmt.Fprintln(os.Stdout, "  -v, --show-nonprinting   use ^ and M- notation, except for LFD and TAB")
}

func (s *state) process(name string) error {
	if name == "-" {
		return s.processReader(os.Stdin)
	}

	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()

	return s.processReader(f)
}

func (s *state) processReader(r io.Reader) error {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if procErr := s.processBytes(buf[:n]); procErr != nil {
				return procErr
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (s *state) processBytes(data []byte) error {
	for i := 0; i < len(data); i++ {
		b := data[i]

		if s.pendingCR {
			if b == '\n' {
				if err := s.writeString("^M"); err != nil {
					return err
				}
				s.pendingCR = false
				s.atLineStart = false
				s.consecutiveNewlines = 0
			} else {
				if err := s.writeByte('\r'); err != nil {
					return err
				}
				s.pendingCR = false
			}
		}

		if b == '\n' {
			s.consecutiveNewlines++
			if s.opts.squeezeBlank && s.consecutiveNewlines > 1 {
				s.atLineStart = true
				continue
			}

			if s.opts.number && !s.opts.numberNonblank && s.atLineStart {
				if err := s.writeLineNumber(); err != nil {
					return err
				}
			}

			if s.opts.showEnds {
				if err := s.writeByte('$'); err != nil {
					return err
				}
			}
			if err := s.writeByte('\n'); err != nil {
				return err
			}
			s.atLineStart = true
			continue
		}

		if s.atLineStart && s.opts.number {
			if !s.opts.numberNonblank {
				if err := s.writeLineNumber(); err != nil {
					return err
				}
			} else {
				if err := s.writeLineNumber(); err != nil {
					return err
				}
			}
		}

		s.atLineStart = false
		s.consecutiveNewlines = 0

		if !s.opts.showNonprinting {
			if s.opts.showEnds && b == '\r' {
				if i+1 < len(data) {
					if data[i+1] == '\n' {
						if err := s.writeString("^M"); err != nil {
							return err
						}
						continue
					}
				} else {
					s.pendingCR = true
					continue
				}
			}

			if s.opts.showTabs && b == '\t' {
				if err := s.writeString("^I"); err != nil {
					return err
				}
				continue
			}

			if err := s.writeByte(b); err != nil {
				return err
			}
			continue
		}

		if b >= 32 {
			if b < 127 {
				if err := s.writeByte(b); err != nil {
					return err
				}
			} else if b == 127 {
				if err := s.writeString("^?"); err != nil {
					return err
				}
			} else {
				if err := s.writeString("M-"); err != nil {
					return err
				}
				if b >= 128+32 {
					if b < 128+127 {
						if err := s.writeByte(b - 128); err != nil {
							return err
						}
					} else {
						if err := s.writeString("^?"); err != nil {
							return err
						}
					}
				} else {
					if err := s.writeByte('^'); err != nil {
						return err
					}
					if err := s.writeByte(b - 128 + 64); err != nil {
						return err
					}
				}
			}
			continue
		}

		if b == '\t' && !s.opts.showTabs {
			if err := s.writeByte('\t'); err != nil {
				return err
			}
			continue
		}

		if b == '\n' {
			continue
		}

		if err := s.writeByte('^'); err != nil {
			return err
		}
		if err := s.writeByte(b + 64); err != nil {
			return err
		}
	}

	return nil
}

func (s *state) writeLineNumber() error {
	if _, err := fmt.Fprintf(s.out, "%6d\t", s.lineNo); err != nil {
		return err
	}
	s.lineNo++
	return nil
}

func (s *state) writeString(v string) error {
	_, err := s.out.WriteString(v)
	return err
}

func (s *state) writeByte(b byte) error {
	return s.out.WriteByte(b)
}
