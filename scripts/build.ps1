$ErrorActionPreference = "Stop"
pnpm install --frozen-lockfile
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
pnpm run build:web
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
go test ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
New-Item -ItemType Directory -Force bin | Out-Null
go build -trimpath -o bin/uniproxy.exe ./cmd/uniproxy
exit $LASTEXITCODE
