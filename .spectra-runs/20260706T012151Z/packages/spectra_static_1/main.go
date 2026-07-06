package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"
)

type catState struct {
	lineCounter int
	newlines    int
	pendingCR   bool
}

type safeWriter struct {
	w *bufio.Writer
}

func (s *safeWriter) WriteString(str string) {
	_, err := s.w.WriteString(str)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cat: write error: %v\n", err)
		os.Exit(1)
	}
}

func (s *safeWriter) WriteByte(b byte) error {
	err := s.w.WriteByte(b)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cat: write error: %v\n", err)
		os.Exit(1)
	}
	return nil
}

func (s *safeWriter) Flush() {
	err := s.w.Flush()
	if err != nil {
		fmt.Fprintf(os.Stderr, "cat: write error: %v\n", err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Printf("Usage: %s [OPTION]... [FILE]...\n", os.Args[0])
	fmt.Printf("Concatenate FILE(s) to standard output.\n\n")
	fmt.Printf("With no FILE, or when FILE is -, read standard input.\n\n")
	fmt.Printf("  -A, --show-all           equivalent to -vET\n")
	fmt.Printf("  -b, --number-nonblank    number nonempty output lines, overrides -n\n")
	fmt.Printf("  -e                       equivalent to -vE\n")
	fmt.Printf("  -E, --show-ends          display $ or ^M$ at end of each line\n")
	fmt.Printf("  -n, --number             number all output lines\n")
	fmt.Printf("  -s, --squeeze-blank      suppress repeated empty output lines\n")
	fmt.Printf("  -t                       equivalent to -vT\n")
	fmt.Printf("  -T, --show-tabs          display TAB characters as ^I\n")
	fmt.Printf("  -u                       (ignored)\n")
	fmt.Printf("  -v, --show-nonprinting   use ^ and M- notation, except for LFD and TAB\n")
	fmt.Printf("      --help        display this help and exit\n")
	fmt.Printf("      --version     output version information and exit\n\n")
	fmt.Printf("Examples:\n")
	fmt.Printf("  %s f - g  Output f's contents, then standard input, then g's contents.\n", os.Args[0])
	fmt.Printf("  %s        Copy standard input to standard output.\n", os.Args[0])
}

func printVersion() {
	fmt.Printf("cat (GNU coreutils) 9.5\n")
	fmt.Printf("Copyright (C) 2026 Free Software Foundation, Inc.\n")
	fmt.Printf("License GPLv3+: GNU GPL version 3 or later <https://gnu.org/licenses/gpl.html>.\n")
	fmt.Printf("This is free software: you are free to change and redistribute it.\n")
	fmt.Printf("There is NO WARRANTY, to the extent permitted by law.\n\n")
	fmt.Printf("Written by Torbjorn Granlund and Richard M. Stallman.\n")
}

func printUsageError(opt string) {
	if strings.HasPrefix(opt, "--") {
		fmt.Fprintf(os.Stderr, "cat: unrecognized option '%s'\n", opt)
	} else {
		fmt.Fprintf(os.Stderr, "cat: invalid option -- '%s'\n", opt)
	}
	fmt.Fprintf(os.Stderr, "Try 'cat --help' for more information.\n")
}

func printError(infile string, err error) {
	if pe, ok := err.(*os.PathError); ok {
		fmt.Fprintf(os.Stderr, "cat: %s: %v\n", infile, pe.Err)
	} else {
		fmt.Fprintf(os.Stderr, "cat: %s: %v\n", infile, err)
	}
}

func checkCycle(infile string, inFile *os.File, outFile *os.File) error {
	fiIn, err := inFile.Stat()
	if err != nil {
		return nil
	}
	fiOut, err := outFile.Stat()
	if err != nil {
		return nil
	}

	statIn, ok1 := fiIn.Sys().(*syscall.Stat_t)
	statOut, ok2 := fiOut.Sys().(*syscall.Stat_t)
	if ok1 && ok2 {
		isSpecialIn := (statIn.Mode&syscall.S_IFMT) == syscall.S_IFIFO || (statIn.Mode&syscall.S_IFMT) == syscall.S_IFSOCK
		isSpecialOut := (statOut.Mode&syscall.S_IFMT) == syscall.S_IFIFO || (statOut.Mode&syscall.S_IFMT) == syscall.S_IFSOCK

		if !isSpecialIn && !isSpecialOut && statIn.Dev == statOut.Dev && statIn.Ino == statOut.Ino {
			inPos, err1 := inFile.Seek(0, io.SeekCurrent)
			if err1 == nil {
				r1, _, errno := syscall.Syscall(syscall.SYS_FCNTL, outFile.Fd(), syscall.F_GETFL, 0)
				var errF error
				if errno != 0 {
					errF = errno
				}
				flags := int(r1)
				var outPos int64
				if errF == nil && (flags&syscall.O_APPEND) != 0 {
					outPos, _ = outFile.Seek(0, io.SeekEnd)
				} else {
					outPos, _ = outFile.Seek(0, io.SeekCurrent)
				}
				if inPos < outPos {
					return fmt.Errorf("%s: input file is output file", infile)
				}
			}
		}
	}
	return nil
}

func simpleCat(infile string, inFile *os.File, outFile *os.File) error {
	buf := make([]byte, 32768)
	for {
		n, err := inFile.Read(buf)
		if n > 0 {
			wlen, werr := outFile.Write(buf[:n])
			if werr != nil || wlen != n {
				fmt.Fprintf(os.Stderr, "cat: write error: %v\n", werr)
				os.Exit(1)
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%v", err)
		}
	}
}

func cat(r *bufio.Reader, w *safeWriter, state *catState, showNonprinting, showTabs, number, numberNonblank, showEnds, squeezeBlank bool) error { 
	var ch byte
	var err error

	readNext := func() (byte, error) {
		return r.ReadByte()
	}

	ch, err = readNext()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}

	for {
		for ch == '\n' {
			state.newlines++
			if state.newlines > 0 {
				if state.newlines >= 2 {
					state.newlines = 2
					if squeezeBlank {
						ch, err = readNext()
						if err == io.EOF {
							return nil
						}
						if err != nil {
							return err
						}
						continue
					}
				}

				if number && !numberNonblank {
					w.WriteString(fmt.Sprintf("%6d\t", state.lineCounter))
					state.lineCounter++
				}
			}

			if showEnds {
				if state.pendingCR {
					w.WriteString("^M")
					state.pendingCR = false
				}
				w.WriteByte('$')
			}

			w.WriteByte('\n')

			ch, err = readNext()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return err
			}
		}

		if state.pendingCR {
			w.WriteByte('\r')
			state.pendingCR = false
		}

		if state.newlines >= 0 && number {
			w.WriteString(fmt.Sprintf("%6d\t", state.lineCounter))
			state.lineCounter++
		}

		if showNonprinting {
			for {
				if ch >= 32 {
					if ch < 127 {
						w.WriteByte(ch)
					} else if ch == 127 {
						w.WriteString("^?")
					} else {
						w.WriteString("M-")
						if ch >= 160 {
							if ch < 255 {
								w.WriteByte(ch - 128)
							} else {
								w.WriteString("^?")
							}
						} else {
							w.WriteByte('^')
							w.WriteByte(ch - 128 + 64)
						}
					}
				} else if ch == '\t' && !showTabs {
					w.WriteByte('\t')
				} else if ch == '\n' {
					state.newlines = -1
					break
				} else {
					w.WriteByte('^')
					w.WriteByte(ch + 64)
				}

				ch, err = readNext()
				if err == io.EOF {
					state.newlines = -1
					return nil
				}
				if err != nil {
					return err
				}
			}
		} else {
			for {
				if ch == '\t' && showTabs {
					w.WriteString("^I")
				} else if ch != '\n' {
					if ch == '\r' && showEnds {
						nextBytes, peekErr := r.Peek(1)
						if peekErr == nil && nextBytes[0] == '\n' {
							w.WriteString("^M")
						} else if peekErr == io.EOF {
							state.pendingCR = true
						} else {
							w.WriteByte('\r')
						}
					} else {
						w.WriteByte(ch)
					}
				} else {
					state.newlines = -1
					break
				}

				ch, err = readNext()
				if err == io.EOF {
					state.newlines = -1
					return nil
				}
				if err != nil {
					return err
				}
			}
		}
	}
}

func main() {
	var files []string
	var number, numberNonblank, squeezeBlank, showNonprinting, showEnds, showTabs bool

	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "--" {
			files = append(files, os.Args[i+1:]...)
			break
		}
		if arg == "-" {
			files = append(files, arg)
			continue
		}
		if strings.HasPrefix(arg, "--") {
			switch arg {
			case "--help":
				printHelp()
				os.Exit(0)
			case "--version":
				printVersion()
				os.Exit(0)
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
			default:
				printUsageError(arg)
				os.Exit(1)
			}
		} else if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			for _, c := range arg[1:] {
				switch c {
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
					showNonprinting = true
					showEnds = true
				case 't':
					showNonprinting = true
					showTabs = true
				case 'u':
					// Ignored
				default:
					printUsageError(string(c))
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

	useFormat := number || showEnds || showNonprinting || showTabs || squeezeBlank

	state := &catState{
		lineCounter: 1,
		newlines:    0,
		pendingCR:   false,
	}

	var writer *safeWriter
	if useFormat {
		writer = &safeWriter{w: bufio.NewWriter(os.Stdout)}
	}

	ok := true
	for _, infile := range files {
		var inFile *os.File
		if infile == "-" {
			inFile = os.Stdin
		} else {
			var err error
			inFile, err = os.Open(infile)
			if err != nil {
				printError(infile, err)
				ok = false
				continue
			}
		}

		fi, err := inFile.Stat()
		if err != nil {
			printError(infile, err)
			if infile != "-" {
				inFile.Close()
			}
			ok = false
			continue
		}

		if fi.IsDir() {
			fmt.Fprintf(os.Stderr, "cat: %s: Is a directory\n", infile)
			if infile != "-" {
				inFile.Close()
			}
			ok = false
			continue
		}

		err = checkCycle(infile, inFile, os.Stdout)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cat: %v\n", err)
			if infile != "-" {
				inFile.Close()
			}
			ok = false
			continue
		}

		if useFormat {
			reader := bufio.NewReader(inFile)
			err = cat(reader, writer, state, showNonprinting, showTabs, number, numberNonblank, showEnds, squeezeBlank)
			if err != nil {
				printError(infile, err)
				ok = false
			}
		} else {
			err = simpleCat(infile, inFile, os.Stdout)
			if err != nil {
				printError(infile, err)
				ok = false
			}
		}

		if infile != "-" {
			inFile.Close()
		}
	}

	if useFormat {
		if state.pendingCR {
			writer.WriteByte('\r')
		}
		writer.Flush()
	}

	if !ok {
		os.Exit(1)
	}
}
