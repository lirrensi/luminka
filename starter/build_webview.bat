@echo off
cd /d "%~dp0"
call npm run build:sdk
call npm run build:icons
go run github.com/tc-hib/go-winres@latest make --in winres/winres.json
go build -tags webview -ldflags "-H windowsgui" -o ..\luminka-starter-webview.exe .
