package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const mdnsServiceType = "_localbeam._tcp"
const mdnsDomain = "local."

var (
	mdnsEmitMu sync.Mutex
	mdnsEmitAt = map[string]time.Time{}
)

func sanitizeInstanceName(hostname string) string {
	s := strings.TrimSpace(hostname)
	if s == "" {
		return "LocalBeam"
	}
	s = strings.ReplaceAll(s, " ", "-")
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

// RegisterMDNS advertises this host as a LocalBeam receiver (DNS-SD / Bonjour).
func RegisterMDNS(hostname string) (*zeroconf.Server, error) {
	instance := sanitizeInstanceName(hostname)
	txt := []string{
		"ver=" + ProtocolVersion,
		fmt.Sprintf("port=%d", FileTransferPort),
	}
	return zeroconf.Register(instance, mdnsServiceType, mdnsDomain, FileTransferPort, txt, nil)
}

func emitMDNSPeer(wailsCtx context.Context, p Peer) {
	mdnsEmitMu.Lock()
	defer mdnsEmitMu.Unlock()
	if t, ok := mdnsEmitAt[p.IP]; ok && time.Since(t) < 2*time.Second {
		return
	}
	mdnsEmitAt[p.IP] = time.Now()
	runtime.EventsEmit(wailsCtx, "peer-found", p)
}

// BrowseMDNS discovers other LocalBeam instances on the LAN (complements UDP broadcast).
func BrowseMDNS(runCtx context.Context, wailsCtx context.Context) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		fmt.Println("zeroconf resolver:", err)
		return
	}
	entries := make(chan *zeroconf.ServiceEntry)
	go func() {
		err := resolver.Browse(runCtx, mdnsServiceType, mdnsDomain, entries)
		if err != nil && runCtx.Err() == nil {
			fmt.Println("zeroconf browse:", err)
		}
	}()
	go func() {
		for entry := range entries {
			if len(entry.AddrIPv4) == 0 {
				continue
			}
			host := entry.HostName
			if host == "" {
				host = entry.Instance
			}
			host = strings.TrimSuffix(strings.TrimSpace(host), ".")
			if host == "" {
				host = "Device"
			}
			port := entry.Port
			if port == 0 {
				port = FileTransferPort
			}
			p := Peer{
				Hostname: host,
				IP:       entry.AddrIPv4[0].String(),
				Port:     port,
				Version:  ProtocolVersion,
			}
			for _, line := range entry.Text {
				if strings.HasPrefix(line, "ver=") {
					p.Version = strings.TrimPrefix(line, "ver=")
				}
			}
			emitMDNSPeer(wailsCtx, p)
		}
	}()
}
