@echo off
cd /d "%~dp0"
call npm run build:sdk
go build -ldflags "-H windowsgui" -o ..\luminka-starter.exe .
