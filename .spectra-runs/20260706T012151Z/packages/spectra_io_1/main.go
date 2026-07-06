package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
)

var (
	newlines = 0
	lineVal  uint64 = 0
)

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
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "--" {
			files = append(files, os.Args[i+1:]...)
			break
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
				fmt.Fprintf(os.Stderr, "%s: unrecognized option '%s'\n", os.Args[0], arg)
				printTryHelp()
				os.Exit(1)
			}
		} else if strings.HasPrefix(arg, "-") && arg != "-" {
			for _, char := range arg[1:] {
				switch char {
				case 'b':
					number = true
					numberNonblank = true
				case 'n':
					number = true
				case 's':
					squeezeBlank = true
				case 'v':
					showNonprinting = true
				case 'E':
					showEnds = true
				case 'T':
					showTabs = true
				case 'A':
					showNonprinting = true
					showEnds = true
					showTabs = true
				case 'e':
					showEnds = true
					showNonprinting = true
				case 't':
					showTabs = true
					showNonprinting = true
				case 'u':
					// Ignored
				default:
					fmt.Fprintf(os.Stderr, "%s: invalid option -- '%c'\n", os.Args[0], char)
					printTryHelp()
					os.Exit(1)
				}
			}
		} else {
			files = append(files, arg)
		}
	}

	if len(files) == 0 {
		files = []string{"-"}
	}

	formatting := number || showEnds || squeezeBlank || showNonprinting || showTabs

	ok := true
	for _, infile := range files {
		var r io.Reader
		var closer io.Closer

		if infile == "-" {
			r = os.Stdin
		} else {
			f, err := os.Open(infile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %s: %s\n", os.Args[0], infile, getStrerror(err))
				ok = false
				continue
			}
			r = f
			closer = f
		}

		if formatting {
			br := bufio.NewReader(r)
			err := catFile(br, os.Stdout, showNonprinting, showTabs, number, numberNonblank, showEnds, squeezeBlank)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %s: %s\n", os.Args[0], infile, getStrerror(err))
				ok = false
			}
		} else {
			_, err := io.Copy(os.Stdout, r)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %s: %s\n", os.Args[0], infile, getStrerror(err))
				ok = false
			}
		}

		if closer != nil {
			closer.Close()
		}
	}

	if !ok {
		os.Exit(1)
	}
}

func printLineNum(w io.Writer) error {
	lineVal++
	_, err := fmt.Fprintf(w, "%6d\t", lineVal)
	return err
}

func catFile(r *bufio.Reader, w io.Writer, showNonprinting, showTabs, number, numberNonblank, showEnds, squeezeBlank bool) error {
	for {
		ch, err := r.ReadByte()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		if ch == '\n' {
			newlines++
			if newlines > 0 {
				if newlines >= 2 {
					newlines = 2
					if squeezeBlank {
						continue
					}
				}
				if number && !numberNonblank {
					if err := printLineNum(w); err != nil {
						return err
					}
				}
			}
			if showEnds {
				if _, err := w.Write([]byte("$")); err != nil {
					return err
				}
			}
			if _, err := w.Write([]byte{'\n'}); err != nil {
				return err
			}
			continue
		}

		if newlines >= 0 && number {
			if err := printLineNum(w); err != nil {
				return err
			}
		}
		newlines = -1

		if showNonprinting {
			if ch >= 32 {
				if ch < 127 {
					if _, err := w.Write([]byte{ch}); err != nil {
						return err
					}
				} else if ch == 127 {
					if _, err := w.Write([]byte("^?")); err != nil {
						return err
					}
				} else {
					if _, err := w.Write([]byte("M-")); err != nil {
						return err
					}
					if ch >= 128+32 {
						if ch < 255 {
							if _, err := w.Write([]byte{ch - 128}); err != nil {
								return err
							}
						} else {
							if _, err := w.Write([]byte("^?")); err != nil {
								return err
							}
						}
					} else {
						if _, err := w.Write([]byte{'^', ch - 128 + 64}); err != nil {
							return err
						}
					}
				}
			} else if ch == '\t' {
				if showTabs {
					if _, err := w.Write([]byte("^I")); err != nil {
						return err
					}
				} else {
					if _, err := w.Write([]byte{'\t'}); err != nil {
						return err
					}
				}
			} else {
				if _, err := w.Write([]byte{'^', ch + 64}); err != nil {
					return err
				}
			}
		} else {
			if ch == '\t' {
				if showTabs {
					if _, err := w.Write([]byte("^I")); err != nil {
						return err
					}
				} else {
					if _, err := w.Write([]byte{'\t'}); err != nil {
						return err
					}
				}
			} else if ch == '\r' {
				if showEnds {
					next, peekErr := r.Peek(1)
					if peekErr == nil && next[0] == '\n' {
						if _, err := w.Write([]byte("^M")); err != nil {
							return err
						}
					} else {
						if _, err := w.Write([]byte{'\r'}); err != nil {
							return err
						}
					}
				} else {
					if _, err := w.Write([]byte{'\r'}); err != nil {
						return err
					}
				}
			} else {
					if _, err := w.Write([]byte{ch}); err != nil {
						return err
					}
			}
		}
	}
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
}

func printVersion() {
	fmt.Printf("cat (GNU coreutils) 9.5 (Go translation)\n")
}

func getStrerror(err error) string {
	if err == nil {
		return ""
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		err = pathErr.Err
	}

	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.ENOENT:
			return "No such file or directory"
		case syscall.EACCES:
			return "Permission denied"
		case syscall.EISDIR:
			return "Is a directory"
		case syscall.ENOTDIR:
			return "Not a directory"
		case syscall.EBADF:
			return "Bad file descriptor"
		case syscall.EEXIST:
			return "File exists"
		case syscall.EINVAL:
			return "Invalid argument"
		case syscall.EIO:
			return "Input/output error"
		case syscall.ELOOP:
			return "Too many levels of symbolic links"
		case syscall.ENAMETOOLONG:
			return "File name too long"
		case syscall.ENOSPC:
			return "No space left on device"
		case syscall.ENOTEMPTY:
			return "Directory not empty"
		case syscall.EPERM:
			return "Operation not permitted"
		case syscall.EPIPE:
			return "Broken pipe"
		}
	}

	s := err.Error()
	if len(s) > 0 {
		return strings.ToUpper(s[:1]) + s[1:]
	}
	return s
}
