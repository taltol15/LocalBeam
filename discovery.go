package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Peer is a discovered device on the LAN (UDP and/or mDNS).
type Peer struct {
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Version  string `json:"version,omitempty"`
}

// DiscoveryService אחראי על מציאת מחשבים
type DiscoveryService struct {
	MyPeer Peer
	Peers  map[string]Peer
}

func NewDiscoveryService() *DiscoveryService {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "Unknown"
	}

	return &DiscoveryService{
		MyPeer: Peer{
			Hostname: hostname,
			IP:       "",
			Port:     FileTransferPort,
			Version:  ProtocolVersion,
		},
		Peers: make(map[string]Peer),
	}
}

// StartBroadcasting שולח הודעות "אני כאן" לרשת
func (d *DiscoveryService) StartBroadcasting() {
	// 1. שידור לכל הרשת (Broadcast) - למחשבים אחרים
	broadcastAddr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("255.255.255.255:%d", BroadcastPort))
	broadcastConn, err := net.DialUDP("udp", nil, broadcastAddr)
	if err != nil {
		fmt.Println("Error broadcasting:", err)
		return
	}
	defer broadcastConn.Close()

	// 2. שידור לוקאלי (Localhost) - כדי שווינדוס יראה את עצמו (תיקון ל-Loopback)
	localAddr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf("127.0.0.1:%d", BroadcastPort))
	localConn, _ := net.DialUDP("udp", nil, localAddr)
	if localConn != nil {
		defer localConn.Close()
	}

	// לולאת השידור האינסופית
	for {
		payload, _ := json.Marshal(d.MyPeer)

		// שליחה החוצה
		broadcastConn.Write(payload)

		// שליחה פנימה (לעצמי)
		if localConn != nil {
			localConn.Write(payload)
		}

		time.Sleep(1 * time.Second)
	}
}

// StartListening מקשיב להודעות ממחשבים אחרים ומעדכן את הממשק
func (d *DiscoveryService) StartListening(ctx context.Context) {
	addr, _ := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", BroadcastPort))
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		fmt.Println("Error listening:", err)
		return
	}
	defer conn.Close()

	fmt.Println("Listening for peers...")
	buf := make([]byte, 1024)
	
	for {
		// קריאת ההודעה
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		// פענוח ה-JSON
		var newPeer Peer
		err = json.Unmarshal(buf[:n], &newPeer)
		if err != nil {
			continue
		}

		newPeer.IP = remoteAddr.IP.String()
		if newPeer.Port == 0 {
			newPeer.Port = FileTransferPort
		}
		if newPeer.Version == "" {
			newPeer.Version = "1"
		}

		// בדיקה אם זה מחשב חדש (רק לצורך הלוג בטרמינל)
		if _, exists := d.Peers[newPeer.IP]; !exists {
			d.Peers[newPeer.IP] = newPeer
			fmt.Printf("New Peer Found: %s (%s)\n", newPeer.Hostname, newPeer.IP)
		}

		// --- התיקון החשוב ---
		// אנחנו שולחים את האירוע לממשק תמיד!
		// ה-Frontend ב-React כבר יודע לסנן כפילויות, אז לא אכפת לנו לשלוח שוב ושוב.
		// זה מבטיח שאם החלון נפתח באיחור, הוא יקבל את המידע בסבב הבא.
		runtime.EventsEmit(ctx, "peer-found", newPeer)
	}
}