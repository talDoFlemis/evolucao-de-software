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
	pendingCr bool
	lineNum   int64
}

func main() {
	var (
		number          bool
		numberNonblank  bool
		squeezeBlank    bool
		showEnds        bool
		showNonprinting bool
		showTabs        bool
		showHelp        bool
		showVersion     bool
	)

	files := []string{}
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "--" {
			files = append(files, os.Args[i+1:]...)
			break
		}
		if arg == "-" {
			files = append(files, "-")
			continue
		}
		if strings.HasPrefix(arg, "--") {
			switch arg {
			case "--show-all":
				showNonprinting = true
				showEnds = true
				showTabs = true
			case "--number-nonblank":
				number = true
				numberNonblank = true
			case "--show-ends":
				showEnds = true
			case "--number":
				number = true
			case "--squeeze-blank":
				squeezeBlank = true
			case "--show-tabs":
				showTabs = true
			case "--show-nonprinting":
				showNonprinting = true
			case "--help":
				showHelp = true
			case "--version":
				showVersion = true
			default:
				fmt.Fprintf(os.Stderr, "%s: unrecognized option '%s'\n", os.Args[0], arg)
				fmt.Fprintf(os.Stderr, "Try '%s --help' for more information.\n", os.Args[0])
				os.Exit(1)
			}
		} else if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			for j := 1; j < len(arg); j++ {
				switch arg[j] {
				case 'A':
					showNonprinting = true
					showEnds = true
					showTabs = true
				case 'b':
					number = true
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
					// ignored
				case 'v':
					showNonprinting = true
				default:
					fmt.Fprintf(os.Stderr, "%s: invalid option -- '%c'\n", os.Args[0], arg[j])
					fmt.Fprintf(os.Stderr, "Try '%s --help' for more information.\n", os.Args[0])
					os.Exit(1)
				}
			}
		} else {
			files = append(files, arg)
		}
	}

	if showHelp {
		usage()
		os.Exit(0)
	}
	if showVersion {
		version()
		os.Exit(0)
	}

	if len(files) == 0 {
		files = []string{"-"}
	}

	stdoutStat, stdoutStatErr := os.Stdout.Stat()

	state := &catState{
		newlines:  0,
		pendingCr: false,
		lineNum:   0,
	}

	bufferedStdout := bufio.NewWriterSize(os.Stdout, 65536)
	defer bufferedStdout.Flush()

	haveReadStdin := false
	ok := true

	for _, infile := range files {
		var r *bufio.Reader
		var file *os.File
		var err error
		var inStat os.FileInfo

		if infile == "-" {
			haveReadStdin = true
			file = os.Stdin
			inStat, err = file.Stat()
			if err != nil {
				inStat = nil
			}
			r = bufio.NewReaderSize(os.Stdin, 65536)
		} else {
			file, err = os.Open(infile)
			if err != nil {
				printError(infile, err)
				ok = false
				continue
			}
			inStat, err = file.Stat()
			if err != nil {
				printError(infile, err)
				file.Close()
				ok = false
				continue
			}
			if inStat.IsDir() {
				fmt.Fprintf(os.Stderr, "%s: %s: Is a directory\n", os.Args[0], infile)
				file.Close()
				ok = false
				continue
			}
			if stdoutStatErr == nil && os.SameFile(stdoutStat, inStat) && stdoutStat.Mode().IsRegular() {
				fmt.Fprintf(os.Stderr, "%s: %s: input file is output file\n", os.Args[0], infile)
				file.Close()
				ok = false
				continue
			}
			r = bufio.NewReaderSize(file, 65536)
		}

		err = runCat(r, file, state, number, numberNonblank, squeezeBlank, showEnds, showNonprinting, showTabs, bufferedStdout)
		if infile != "-" {
			file.Close()
		}
		if err != nil {
			printError(infile, err)
			ok = false
		}
	}

	if state.pendingCr {
		bufferedStdout.WriteByte('\r')
	}

	haveReadStdin = haveReadStdin

	if !ok {
		os.Exit(1)
	}
}

func printError(infile string, err error) {
	if pathErr, ok := err.(*os.PathError); ok {
		fmt.Fprintf(os.Stderr, "%s: %s: %v\n", os.Args[0], infile, pathErr.Err)
	} else {
		fmt.Fprintf(os.Stderr, "%s: %s: %v\n", os.Args[0], infile, err)
	}
}

func runCat(r *bufio.Reader, file *os.File, state *catState, number, numberNonblank, squeezeBlank, showEnds, showNonprinting, showTabs bool, bufferedStdout *bufio.Writer) error {
	if !(number || showEnds || showNonprinting || showTabs || squeezeBlank) {
		if err := bufferedStdout.Flush(); err != nil {
			return err
		}
		_, err := io.Copy(os.Stdout, file)
		return err
	}

	for {
		if r.Buffered() == 0 {
			if err := bufferedStdout.Flush(); err != nil {
				return err
			}
		}

		b, err := r.ReadByte()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		if b == '\n' {
			state.newlines++
			if state.newlines >= 2 {
				state.newlines = 2
				if squeezeBlank {
					continue
				}
			}
			if state.newlines > 0 {
				if number && !numberNonblank {
					printLineNum(bufferedStdout, state)
				}
			}
			if showEnds {
				if state.pendingCr {
					if _, err := bufferedStdout.Write([]byte{'^', 'M'}); err != nil {
						return err
					}
					state.pendingCr = false
				}
				if _, err := bufferedStdout.Write([]byte{'$'}); err != nil {
					return err
				}
			}
			if err := bufferedStdout.WriteByte('\n'); err != nil {
				return err
			}
		} else {
			if state.pendingCr {
				if err := bufferedStdout.WriteByte('\r'); err != nil {
					return err
				}
				state.pendingCr = false
			}
			if state.newlines >= 0 && number {
				printLineNum(bufferedStdout, state)
			}
			state.newlines = -1

			if showEnds && !showNonprinting && b == '\r' {
				next, err := r.Peek(1)
				if err == nil && next[0] == '\n' {
					state.pendingCr = true
					continue
				}
				if err == io.EOF {
					state.pendingCr = true
					continue
				}
			}

			if err := writeChar(b, bufferedStdout, showNonprinting, showTabs); err != nil {
				return err
			}
		}
	}
	return nil
}

func printLineNum(w io.Writer, state *catState) {
	state.lineNum++
	fmt.Fprintf(w, "%6d\t", state.lineNum)
}

func writeChar(b byte, w io.Writer, showNonprinting, showTabs bool) error {
	if showNonprinting {
		if b >= 32 {
			if b < 127 {
				_, err := w.Write([]byte{b})
				return err
			} else if b == 127 {
				_, err := w.Write([]byte{'^', '?'})
				return err
			} else {
				if _, err := w.Write([]byte{'M', '-'}); err != nil {
					return err
				}
				if b >= 128+32 {
					if b < 128+127 {
						_, err := w.Write([]byte{b - 128})
						return err
					} else {
						_, err := w.Write([]byte{'^', '?'})
						return err
					}
				} else {
					_, err := w.Write([]byte{'^', b - 128 + 64})
					return err
				}
			}
		} else if b == '\t' && !showTabs {
			_, err := w.Write([]byte{'\t'})
			return err
		} else if b == '\n' {
			_, err := w.Write([]byte{'\n'})
			return err
		} else {
			_, err := w.Write([]byte{'^', b + 64})
			return err
		}
	} else {
		if b == '\t' && showTabs {
			_, err := w.Write([]byte{'^', 'I'})
			return err
		}
		_, err := w.Write([]byte{b})
		return err
	}
}

func usage() {
	fmt.Printf("Usage: %s [OPTION]... [FILE]...\n", os.Args[0])
	fmt.Println("Concatenate FILE(s) to standard output.")
	fmt.Println()
	fmt.Println("With no FILE, or when FILE is -, read standard input.")
	fmt.Println()
	fmt.Println("  -A, --show-all           equivalent to -vET")
	fmt.Println("  -b, --number-nonblank    number nonempty output lines, overrides -n")
	fmt.Println("  -e                       equivalent to -vE")
	fmt.Println("  -E, --show-ends          display $ at end of each line")
	fmt.Println("  -n, --number             number all output lines")
	fmt.Println("  -s, --squeeze-blank      suppress repeated empty output lines")
	fmt.Println("  -t                       equivalent to -vT")
	fmt.Println("  -T, --show-tabs          display TAB characters as ^I")
	fmt.Println("  -u                       (ignored)")
	fmt.Println("  -v, --show-nonprinting   use ^ and M- notation, except for LFD and TAB")
	fmt.Println("      --help        display this help and exit")
	fmt.Println("      --version     output version information and exit")
	fmt.Println()
	fmt.Println("Examples:\n")
	fmt.Printf("  %s f - g  Output f's contents, then standard input, then g's contents.\n", os.Args[0])
	fmt.Printf("  %s        Copy standard input to standard output.\n", os.Args[0])
}

func version() {
	fmt.Println("cat (GNU coreutils) Go clone")
	fmt.Println("Copyright (C) 2026 Free Software Foundation, Inc.")
	fmt.Println("License GPLv3+: GNU GPL version 3 or later <https://gnu.org/licenses/gpl.html>.")
	fmt.Println("This is free software: you are free to change and redistribute.  There is NO WARRANTY.\n")
	fmt.Println("Written by Torbjorn Granlund and Richard M. Stallman.")
}
