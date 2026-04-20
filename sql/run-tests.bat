@echo off
echo Running gofmt...
for /R %%f in (*.go) do (
    gofmt -w "%%f"
    if errorlevel 1 (
        echo gofmt failed
        exit /b 1
    )
)

echo Running tests...
go test ./...
if errorlevel 1 (
    echo Tests failed
    exit /b 1
)

echo Running build...
go build ./...
if errorlevel 1 (
    echo Build failed
    exit /b 1
)

echo All checks passed!
