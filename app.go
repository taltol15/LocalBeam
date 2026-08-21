package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/grandcat/zeroconf"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx       context.Context
	runCtx    context.Context
	runCancel context.CancelFunc
	discovery *DiscoveryService
	mdnsSrv   *zeroconf.Server
}

func NewApp() *App {
	return &App{
		discovery: NewDiscoveryService(),
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.runCtx, a.runCancel = context.WithCancel(context.Background())

	StartFileServer(ctx)
	a.discovery.MyPeer.Fingerprint = GetCertFingerprint()
	a.discovery.MyPeer.Version = ProtocolVersion

	go a.discovery.StartBroadcasting()
	go a.discovery.StartListening(ctx)

	if srv, err := RegisterMDNS(a.discovery.MyPeer.Hostname); err != nil {
		fmt.Println("mDNS register:", err)
	} else {
		a.mdnsSrv = srv
	}

	BrowseMDNS(a.runCtx, ctx)
}

func (a *App) shutdown(_ context.Context) {
	if a.runCancel != nil {
		a.runCancel()
	}
	if a.mdnsSrv != nil {
		a.mdnsSrv.Shutdown()
		a.mdnsSrv = nil
	}
	ShutdownFileServer()
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

func (a *App) SendFile(address, filePath, pinCode, senderName, senderEmail string) string {
	if err := SendFileToPeer(address, filePath, pinCode, senderName, senderEmail); err != nil {
		return "Error: " + err.Error()
	}
	return "Success"
}

func (a *App) GetMyPIN() string {
	return GetCurrentPIN()
}

func (a *App) GetFingerprint() string {
	return GetCertFingerprint()
}

func (a *App) ProtocolInfo() string {
	return ProtocolVersion
}

func (a *App) RespondToOffer(offerID string, accept bool) string {
	return RespondToOffer(offerID, accept)
}

func (a *App) GetAuditLog() string {
	entries, err := ReadAuditLog(50)
	if err != nil {
		return "[]"
	}
	b, err := json.Marshal(entries)
	if err != nil {
		return "[]"
	}
	return string(b)
}
