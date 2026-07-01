You are an isolated SPECTRA translation agent.

Task: translate context/cat.c into a standalone idiomatic Go command-line program using exactly one validated specification modality.

Inputs available in this package directory:
- context/cat.c: source code to translate
- context/spec.md: the io SPECTRA specification for this candidate

Output requirements:
- Create a new Go package in the current directory.
- Use package main.
- Write go.mod and one or more .go files.
- Do not use cgo.
- Do not use third-party dependencies.
- Do not edit files under context/.
- The command should be compatible with GNU cat for the implemented behavior.
- Implement the options visible in cat.c: -A, -b, -e, -E, -n, -s, -t, -T, -u, -v and their long forms.
- Preserve state across multiple input files where cat.c does so.
- The program must build with: go build .

SPECTRA rule: use the attached io spec to guide translation. Do not combine other spec modalities.
