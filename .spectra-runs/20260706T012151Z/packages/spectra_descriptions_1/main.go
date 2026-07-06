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
	lineNum       int
	newlines      int
	pendingCR     bool
	haveReadStdin bool
}

func main() {
	var (
		showAll         bool
		numberNonblank  bool
		showEnds        bool
		number          bool
		squeezeBlank    bool
		showTabs        bool
		showNonprinting bool
		help            bool
		version         bool
	)

	progName := "cat"
	if len(os.Args) > 0 {
		progName = os.Args[0]
	}

	var files []string
	args := os.Args[1:]

	for i := 0; i < len(args); {
		arg := args[i]
		if arg == "--" {
			files = append(files, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			if strings.HasPrefix(arg, "--") {
				opt := arg[2:]
				switch opt {
				case "show-all":
					showAll = true
				case "number-nonblank":
					numberNonblank = true
				case "show-ends":
					showEnds = true
				case "number":
					number = true
				case "squeeze-blank":
					squeezeBlank = true
				case "show-tabs":
					showTabs = true
				case "show-nonprinting":
					showNonprinting = true
				case "help":
					help = true
				case "version":
					version = true
				default:
					fmt.Fprintf(os.Stderr, "%s: unrecognized option '%s'\n", progName, arg)
					printTryHelp(progName)
					os.Exit(1)
				}
			} else {
				for j := 1; j < len(arg); j++ {
					c := arg[j]
					switch c {
					case 'A':
						showAll = true
					case 'b':
						numberNonblank = true
					case 'e':
						showEnds = true
						showNonprinting = true
					case 'E':
						showEnds = true
					case 'n':
						number = true
					case 's':
						squeezeBlank = true
					case 't':
						showTabs = true
						showNonprinting = true
					case 'T':
						showTabs = true
					case 'u':
						// Ignored
					case 'v':
						showNonprinting = true
					default:
						fmt.Fprintf(os.Stderr, "%s: invalid option -- '%c'\n", progName, c)
						printTryHelp(progName)
						os.Exit(1)
					}
				}
			}
			i++
		} else {
			files = append(files, arg)
			i++
		}
	}

	if help {
		printHelp(progName)
		os.Exit(0)
	}
	if version {
		printVersion()
		os.Exit(0)
	}

	if showAll {
		showNonprinting = true
		showEnds = true
		showTabs = true
	}
	if numberNonblank {
		number = true
	}

	ostat, err := os.Stdout.Stat()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: standard output: %v\n", progName, err)
		os.Exit(1)
	}

	state := &catState{}

	if len(files) == 0 {
		files = []string{"-"}
	}

	ok := true
	for _, infile := range files {
		var file *os.File
		var readingStdin bool

		if infile == "-" {
			readingStdin = true
			state.haveReadStdin = true
			file = os.Stdin
		} else {
			var err error
			file, err = os.Open(infile)
			if err != nil {
				if pathErr, ok := err.(*os.PathError); ok {
					fmt.Fprintf(os.Stderr, "%s: %s: %v\n", progName, infile, pathErr.Err)
				} else {
					fmt.Fprintf(os.Stderr, "%s: %s: %v\n", progName, infile, err)
				}
				ok = false
				continue
			}
		}

		istat, err := file.Stat()
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %s: %v\n", progName, infile, err)
			if !readingStdin {
				file.Close()
			}
			ok = false
			continue
		}

		if isInputFileOutputFile(file, istat, ostat) {
			fmt.Fprintf(os.Stderr, "%s: %s: input file is output file\n", progName, infile)
			if !readingStdin {
				file.Close()
			}
			ok = false
			continue
		}

		err = state.catFile(file, os.Stdout, showNonprinting, showTabs, number, numberNonblank, showEnds, squeezeBlank)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %s: %v\n", progName, infile, err)
			ok = false
		}

		if !readingStdin {
			file.Close()
		}
	}

	if state.pendingCR {
		os.Stdout.Write([]byte{'\r'})
	}

	if state.haveReadStdin {
		if err := os.Stdin.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "%s: closing standard input: %v\n", progName, err)
			os.Exit(1)
		}
	}

	if !ok {
		os.Exit(1)
	}
}

func printTryHelp(progName string) {
	fmt.Fprintf(os.Stderr, "Try '%s --help' for more information.\n", progName)
}

func printHelp(progName string) {
	fmt.Printf("Usage: %s [OPTION]... [FILE]...\n", progName)
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
	fmt.Println("      --help        display this help and exit")
	fmt.Println("      --version     output version information and exit")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Printf("  %s f - g  Output f's contents, then standard input, then g's contents.\n", progName)
	fmt.Printf("  %s        Copy standard input to standard output.\n", progName)
}

func printVersion() {
	fmt.Println("cat (GNU coreutils) 9.5")
	fmt.Println("Copyright (C) 2026 Free Software Foundation, Inc.")
	fmt.Println("License GPLv3+: GNU GPL version 3 or later <https://gnu.org/licenses/gpl.html>.")
	fmt.Println("This is free software: you are free to change and redistribute it.")
	fmt.Println("There is NO WARRANTY, to the extent permitted by law.")
	fmt.Println()
	fmt.Println("Written by Torbjorn Granlund and Richard M. Stallman.")
}

func isInputFileOutputFile(file *os.File, istat, ostat os.FileInfo) bool {
	if !os.SameFile(istat, ostat) {
		return false
	}
	if !istat.Mode().IsRegular() {
		return false
	}
	inPos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return false
	}

	fd := int(os.Stdout.Fd())
	flags, err := syscall.FcntlInt(fd, syscall.F_GETFL, 0)
	var outPos int64
	if err == nil && (flags&syscall.O_APPEND) != 0 {
		outPos = ostat.Size()
	} else {
		outPos, err = os.Stdout.Seek(0, io.SeekCurrent)
		if err != nil {
			return false
		}
	}

	return inPos < outPos
}

func (s *catState) catFile(r io.Reader, w io.Writer, showNonprinting, showTabs, number, numberNonblank, showEnds, squeezeBlank bool) error {
	if !number && !showEnds && !showNonprinting && !showTabs && !squeezeBlank {
		_, err := io.Copy(w, r)
		return err
	}

	bufReader := bufio.NewReader(r)
	bufWriter := bufio.NewWriter(w)
	defer bufWriter.Flush()

	for {
		ch, err := bufReader.ReadByte()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if ch == '\n' {
			s.newlines++
			if s.newlines > 0 {
				if s.newlines >= 2 {
					s.newlines = 2
					if squeezeBlank {
						continue
					}
				}
				if number && !numberNonblank {
					s.lineNum++
					fmt.Fprintf(bufWriter, "%6d\t", s.lineNum)
				}
			}
			if showEnds {
				if s.pendingCR {
					bufWriter.WriteString("^M")
					s.pendingCR = false
				}
				bufWriter.WriteByte('$')
			}
			bufWriter.WriteByte('\n')
		} else {
			if s.pendingCR {
				bufWriter.WriteByte('\r')
				s.pendingCR = false
			}
			if c := ch; c == '\r' && showEnds && !showNonprinting {
				next, err := bufReader.Peek(1)
				if err == nil && next[0] == '\n' {
					_, _ = bufReader.ReadByte()
					s.pendingCR = true
					continue
				} else if err == io.EOF || (err != nil && err != bufio.ErrBufferFull) {
					s.pendingCR = true
					continue
				}
			}
			if s.newlines >= 0 && number {
				s.lineNum++
				fmt.Fprintf(bufWriter, "%6d\t", s.lineNum)
			}
			s.newlines = -1

			if showNonprinting {
				if ch >= 32 {
					if ch < 127 {
						bufWriter.WriteByte(ch)
					} else if ch == 127 {
						bufWriter.WriteString("^?")
					} else {
						bufWriter.WriteString("M-")
						if ch >= 128+32 {
							if ch < 128+127 {
								bufWriter.WriteByte(ch - 128)
							} else {
								bufWriter.WriteString("^?")
							}
						} else {
							bufWriter.WriteByte('^')
							bufWriter.WriteByte(ch - 128 + 64)
						}
					}
				} else if ch == '\t' && !showTabs {
					bufWriter.WriteByte('\t')
				} else {
					bufWriter.WriteByte('^')
					bufWriter.WriteByte(ch + 64)
				}
			} else {
				if ch == '\t' && showTabs {
					bufWriter.WriteString("^I")
				} else {
					bufWriter.WriteByte(ch)
				}
			}
		}
	}

	return nil
}
