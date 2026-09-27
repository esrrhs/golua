@echo off
setlocal
cd /d %~dp0

where nex >nul 2>nul
if %errorlevel% neq 0 (
    if exist "tool\nex.exe" (
        set "NEX_CMD=tool\nex.exe"
    ) else (
        echo Installing nex...
        go install github.com/blynn/nex@latest
        set "NEX_CMD=nex"
    )
) else (
    set "NEX_CMD=nex"
)

where goyacc >nul 2>nul
if %errorlevel% neq 0 (
    if exist "tool\goyacc.exe" (
        set "GOYACC_CMD=tool\goyacc.exe"
    ) else (
        echo Installing goyacc...
        go install golang.org/x/tools/cmd/goyacc@latest
        set "GOYACC_CMD=goyacc"
    )
) else (
    set "GOYACC_CMD=goyacc"
)

%NEX_CMD% lex.nex
%GOYACC_CMD% -o yacc.go yacc.y
echo Code generation completed successfully.
