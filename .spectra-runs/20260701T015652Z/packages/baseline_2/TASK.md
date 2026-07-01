You are an isolated baseline translation agent.

Task: translate context/cat.c into a standalone idiomatic Go command-line program.

Inputs available in this package directory:
- context/cat.c: the only source context you may use

Output requirements:
- Create a new Go package in the current directory.
- Use package main.
- Write go.mod and one or more .go files.
- Do not use cgo.
- Do not use third-party dependencies.
- Do not edit files under context/.
- The command should be compatible with GNU cat for the implemented behavior.
- Implement the options visible in cat.c: -A, -b, -e, -E, -n, -s, -t, -T, -u, -v and their long forms.
- The program must build with: go build .

Important: this is the baseline arm. Do not use generated SPECTRA specifications.
