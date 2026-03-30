call npm run build:sdk
go build -tags webview -ldflags "-H windowsgui" -o ..\..\luminka-kanban-webview.exe .
