package main

import (
	"context"
	"fmt"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx       context.Context
	discovery *DiscoveryService
}

func NewApp() *App {
	return &App{
		discovery: NewDiscoveryService(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	go a.discovery.StartBroadcasting()
	go a.discovery.StartListening(ctx)
	go StartFileServer(ctx)
}

func (a *App) SelectFile() string {
	selection, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Choose a file to send",
	})
	if err != nil || selection == "" {
		return ""
	}
	return selection
}

// SendFile - עכשיו מקבל גם pinCode
func (a *App) SendFile(ip string, filePath string, pinCode string) string {
	fmt.Printf("Sending file to %s with PIN %s\n", ip, pinCode)
	
	err := SendFileToPeer(ip, filePath, pinCode)
	if err != nil {
		return "Error: " + err.Error()
	}
	
	return "Success" // הודעה קצרה כדי שהפרונט יזהה הצלחה
}

// GetMyPIN - מחזיר את הקוד הסודי של המחשב הזה
func (a *App) GetMyPIN() string {
	return GetCurrentPIN()
}