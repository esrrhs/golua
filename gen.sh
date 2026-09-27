#!/usr/bin/env bash
set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
cd "$DIR"

export PATH="$PATH:$(go env GOPATH)/bin"

if ! command -v nex >/dev/null 2>&1; then
    echo "Installing nex..."
    go install github.com/blynn/nex@latest
fi

if ! command -v goyacc >/dev/null 2>&1; then
    echo "Installing goyacc..."
    go install golang.org/x/tools/cmd/goyacc@latest
fi

echo "Generating lexer (nex)..."
nex lex.nex

echo "Generating parser (goyacc)..."
goyacc -o yacc.go yacc.y

echo "Code generation completed successfully."
