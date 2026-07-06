package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

var (
	number          bool
	numberNonblank  bool
	squeezeBlank    bool
	showEnds        bool
	showNonprinting bool
	showTabs        bool

	newlines  int   = 0
	pendingCr bool  = false
	lineNum   int64 = 0
	bw        *bufio.Writer
)

func main() {
	var files []string
	stopOptions := false

	for _, arg := range os.Args[1:] {
		if stopOptions {
			files = append(files, arg)
			continue
		}
		if arg == "--" {
			stopOptions = true
			continue
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
			case "--show-all":
				showNonprinting = true
				showEnds = true
				showTabs = true
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
			default:
				fmt.Fprintf(os.Stderr, "%s: unrecognized option '%s'\n", os.Args[0], arg)
				fmt.Fprintf(os.Stderr, "Try '%s --help' for more information.\n", os.Args[0])
				os.Exit(1)
			}
			continue
		}
		if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			for _, c := range arg[1:] {
				switch c {
				case 'b':
					number = true
					numberNonblank = true
				case 'e':
					showEnds = true
					showNonprinting = true
				case 'n':
					number = true
				case 's':
					squeezeBlank = true
				case 't':
					showTabs = true
					showNonprinting = true
				case 'u':
					// Ignored
				case 'v':
					showNonprinting = true
				case 'A':
					showNonprinting = true
					showEnds = true
					showTabs = true
				case 'E':
					showEnds = true
				case 'T':
					showTabs = true
				default:
					fmt.Fprintf(os.Stderr, "%s: invalid option -- '%c'\n", os.Args[0], c)
					fmt.Fprintf(os.Stderr, "Try '%s --help' for more information.\n", os.Args[0])
				os.Exit(1)
				}
			}
			continue
		}
		files = append(files, arg)
	}

	if len(files) == 0 {
		files = []string{"-trim"}
		files[0] = "-"
	}

	ostat, _ := os.Stdout.Stat()

	bw = bufio.NewWriter(os.Stdout)
	defer func() {
		flushWrite()
		if pendingCr {
			if _, err := os.Stdout.Write([]byte{'\r'}); err != nil {
				fmt.Fprintf(os.Stderr, "%s: write error: %v\n", os.Args[0], err)
				os.Exit(1)
			}
		}
	}()

	ok := true
	for _, file := range files {
		if !processFile(file, ostat) {
			ok = false
		}
	}

	if !ok {
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Printf("Usage: %s [OPTION]... [FILE]...\n", os.Args[0])
	fmt.Println("Concatenate FILE(s) to standard output.")
	fmt.Println("\nWith no FILE, or when FILE is -, read standard input.")
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
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Printf("  %s f - g  Output f's contents, then standard input, then g's contents.\n", os.Args[0])
	fmt.Printf("  %s        Copy standard input to standard output.\n", os.Args[0])
}

func printVersion() {
	fmt.Println("cat (GNU coreutils) 9.1")
	fmt.Println("Copyright (C) 2026 Free Software Foundation, Inc.")
	fmt.Println("License GPLv3+: GNU GPL version 3 or later <https://gnu.org/licenses/gpl.html>.")
	fmt.Println("This is free software: you are free to change and redistribute.")
	fmt.Println("There is NO WARRANTY, to the extent permitted by law.")
	fmt.Println()
	fmt.Println("Written by Torbjorn Granlund and Richard M. Stallman.")
}

func flushWrite() {
	if err := bw.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "%s: write error: %v\n", os.Args[0], err)
		os.Exit(1)
	}
}

func writeByte(b byte) {
	if err := bw.WriteByte(b); err != nil {
		fmt.Fprintf(os.Stderr, "%s: write error: %v\n", os.Args[0], err)
		os.Exit(1)
	}
}

func writeString(s string) {
	if _, err := bw.WriteString(s); err != nil {
		fmt.Fprintf(os.Stderr, "%s: write error: %v\n", os.Args[0], err)
		os.Exit(1)
	}
}

func printLineNumber() {
	lineNum++
	writeString(fmt.Sprintf("%6d\t", lineNum))
}

func processFile(file string, ostat os.FileInfo) bool {
	var r io.Reader
	var f *os.File
	if file == "-" {
		r = os.Stdin
	} else {
		var err error
		f, err = os.Open(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %s: %v\n", os.Args[0], file, err)
			return false
		}
		defer f.Close()

		if ostat != nil {
			istat, err := f.Stat()
			if err == nil && os.SameFile(istat, ostat) {
				fmt.Fprintf(os.Stderr, "%s: %s: input file is output file\n", os.Args[0], file)
				return false
			}
		}
		r = f
	}

	if !(number || showEnds || showNonprinting || showTabs || squeezeBlank) {
		return simpleCat(r, file)
	}

	return cat(r, file)
}

func simpleCat(r io.Reader, file string) bool {
	buf := make([]byte, 32*1024)
	for {
		n, rErr := r.Read(buf)
		if n > 0 {
			_, wErr := os.Stdout.Write(buf[:n])
			if wErr != nil {
				fmt.Fprintf(os.Stderr, "%s: write error: %v\n", os.Args[0], wErr)
				os.Exit(1)
			}
		}
		if rErr != nil {
			if rErr == io.EOF {
				return true
			}
			fmt.Fprintf(os.Stderr, "%s: %s: %v\n", os.Args[0], file, rErr)
			return false
		}
	}
}

func cat(r io.Reader, file string) bool {
	br := bufio.NewReader(r)

	for {
		if br.Buffered() == 0 {
			flushWrite()
		}

		ch, err := br.ReadByte()
		if err != nil {
			if err == io.EOF {
				return true
			}
			fmt.Fprintf(os.Stderr, "%s: %s: %v\n", os.Args[0], file, err)
			return false
		}

		if ch != '\n' {
			if pendingCr {
				writeByte('\r')
				pendingCr = false
			}

			if newlines >= 0 && number {
				printLineNumber()
			}

			for {
				if ch == '\n' {
					newlines = -1
					break
				}

				if showNonprinting {
					if ch >= 32 {
						if ch < 127 {
							writeByte(ch)
						} else if ch == 127 {
							writeString("^?")
						} else {
							writeString("M-")
							if ch >= 128+32 {
								if ch < 128+127 {
									writeByte(ch - 128)
								} else {
									writeString("^?")
								}
							} else {
								writeByte('^')
								writeByte(ch - 128 + 64)
							}
						}
					} else if ch == '\t' && !showTabs {
						writeByte('\t')
					} else {
						writeByte('^')
						writeByte(ch + 64)
					}
				} else {
					if ch == '\t' && showTabs {
						writeString("^I")
					} else {
						if ch == '\r' && showEnds {
							if br.Buffered() > 0 {
								nextByte, err := br.Peek(1)
								if err == nil && len(nextByte) > 0 && nextByte[0] == '\n' {
									writeString("^M")
								} else {
									writeByte('\r')
								}
							} else {
								pendingCr = true
							}
						} else {
							writeByte(ch)
						}
					}
				}

				if br.Buffered() == 0 {
					flushWrite()
				}
				ch, err = br.ReadByte()
				if err != nil {
					if err == io.EOF {
						return true
					}
					fmt.Fprintf(os.Stderr, "%s: %s: %v\n", os.Args[0], file, err)
					return false
				}
			}
		}

		newlines++
		if newlines > 0 {
			if newlines >= 2 {
				newlines = 2
				if squeezeBlank {
					continue
				}
			}
			if number && !numberNonblank {
				printLineNumber()
			}
		}

		if showEnds {
				if pendingCr {
					writeString("^M")
					pendingCr = false
				}
				writeByte('$')
		}
		writeByte('\n')
	}
}
