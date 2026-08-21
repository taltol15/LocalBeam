package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	appContext     context.Context
	currentPIN     string
	currentSalt    []byte
	tlsCert        tls.Certificate
	certFingerprint string
	fileServerMu   sync.Mutex
	fileHTTPServer *http.Server

	offersMu     sync.Mutex
	pendingOffers = map[string]*pendingOffer{}

	tokensMu     sync.Mutex
	uploadTokens = map[string]*uploadGrant{}

	challengesMu sync.Mutex
	challenges   = map[string]time.Time{}
)

type pendingOffer struct {
	ID          string
	TransferID  string
	SenderName  string
	SenderEmail string
	Filename    string
	Size        int64
	PeerIP      string
	PIN         string
	Salt        []byte
	CreatedAt   time.Time
	resultCh    chan offerDecision
}

type offerDecision struct {
	accept bool
	token  string
}

type uploadGrant struct {
	TransferID  string
	SenderName  string
	SenderEmail string
	Filename    string
	Size        int64
	PeerIP      string
	PIN         string
	Salt        []byte
	Expires     time.Time
}

type challengeResponse struct {
	Nonce       string `json:"nonce"`
	Salt        string `json:"salt"`
	Fingerprint string `json:"fingerprint"`
	Version     string `json:"version"`
	Port        int    `json:"port"`
}

type offerRequest struct {
	TransferID  string `json:"transfer_id"`
	Challenge   string `json:"challenge"`
	PinProof    string `json:"pin_proof"`
	SenderName  string `json:"sender_name"`
	SenderEmail string `json:"sender_email"`
	Filename    string `json:"filename"`
	Size        int64  `json:"size"`
}

type offerAccepted struct {
	Status string `json:"status"`
	Token  string `json:"token"`
}

type IncomingOfferEvent struct {
	ID          string `json:"id"`
	TransferID  string `json:"transfer_id"`
	SenderName  string `json:"sender_name"`
	SenderEmail string `json:"sender_email"`
	Filename    string `json:"filename"`
	Size        int64  `json:"size"`
	SizeLabel   string `json:"size_label"`
	PeerIP      string `json:"peer_ip"`
	Tip         string `json:"tip"`
}

const verifySenderTip = "Verify with the sender that they are the intended sender of this file."

type ProgressReader struct {
	Reader   io.Reader
	Total    int64
	Current  int64
	Context  context.Context
	LastPerc int
}

func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	pr.Current += int64(n)
	if pr.Total > 0 {
		percent := int(float64(pr.Current) / float64(pr.Total) * 100)
		if percent > pr.LastPerc {
			pr.LastPerc = percent
			if pr.Context != nil {
				runtime.EventsEmit(pr.Context, "upload-progress", percent)
			}
		}
	}
	return n, err
}

func rotatePIN() {
	currentPIN = generatePIN()
	salt, err := newSalt()
	if err != nil {
		salt = make([]byte, saltLen)
	}
	currentSalt = salt
}

func GetCurrentPIN() string {
	return currentPIN
}

func GetCertFingerprint() string {
	return certFingerprint
}

func StartFileServer(ctx context.Context) {
	appContext = ctx
	rotatePIN()

	cert, fp, err := generateSelfSignedTLS()
	if err != nil {
		fmt.Println("tls cert:", err)
		return
	}
	tlsCert = cert
	certFingerprint = fp

	mux := http.NewServeMux()
	mux.HandleFunc("/localbeam/ping", pingHandler)
	mux.HandleFunc("/localbeam/challenge", challengeHandler)
	mux.HandleFunc("/offer", offerHandler)
	mux.HandleFunc("/offer/wait", waitOfferDecisionHandler)
	mux.HandleFunc("/upload", uploadHandler)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", FileTransferPort),
		Handler:           mux,
		ReadHeaderTimeout: 60 * time.Second,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{tlsCert},
			MinVersion:   tls.VersionTLS12,
		},
	}

	fileServerMu.Lock()
	fileHTTPServer = srv
	fileServerMu.Unlock()

	go func() {
		fmt.Printf("LocalBeam HTTPS listening on :%d (fp %s)\n", FileTransferPort, certFingerprint)
		ln, err := net.Listen("tcp", srv.Addr)
		if err != nil {
			fmt.Println("file server listen:", err)
			return
		}
		tlsLn := tls.NewListener(ln, srv.TLSConfig)
		if err := srv.Serve(tlsLn); err != nil && err != http.ErrServerClosed {
			fmt.Println("file server:", err)
		}
	}()
}

func ShutdownFileServer() {
	fileServerMu.Lock()
	srv := fileHTTPServer
	fileHTTPServer = nil
	fileServerMu.Unlock()
	if srv == nil {
		return
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

func pingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set(HeaderLocalBeamVer, ProtocolVersion)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":          true,
		"version":     ProtocolVersion,
		"port":        FileTransferPort,
		"fingerprint": certFingerprint,
	})
}

func challengeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	nonce, err := newChallengeNonce()
	if err != nil {
		http.Error(w, "challenge failed", http.StatusInternalServerError)
		return
	}
	challengesMu.Lock()
	challenges[nonce] = time.Now().Add(3 * time.Minute)
	// prune
	now := time.Now()
	for k, exp := range challenges {
		if now.After(exp) {
			delete(challenges, k)
		}
	}
	challengesMu.Unlock()

	w.Header().Set(HeaderLocalBeamVer, ProtocolVersion)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(challengeResponse{
		Nonce:       nonce,
		Salt:        b64Encode(currentSalt),
		Fingerprint: certFingerprint,
		Version:     ProtocolVersion,
		Port:        FileTransferPort,
	})
}

func offerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req offerRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	req.SenderName = strings.TrimSpace(req.SenderName)
	req.SenderEmail = strings.TrimSpace(strings.ToLower(req.SenderEmail))
	req.Filename = filepath.Base(strings.TrimSpace(req.Filename))
	if req.TransferID == "" || req.Challenge == "" || req.PinProof == "" ||
		req.SenderName == "" || req.SenderEmail == "" || req.Filename == "" || req.Size <= 0 {
		http.Error(w, "missing fields", http.StatusBadRequest)
		return
	}
	if !looksLikeEmail(req.SenderEmail) {
		http.Error(w, "invalid email", http.StatusBadRequest)
		return
	}

	challengesMu.Lock()
	exp, ok := challenges[req.Challenge]
	if ok {
		delete(challenges, req.Challenge)
	}
	challengesMu.Unlock()
	if !ok || time.Now().After(exp) {
		http.Error(w, "challenge expired", http.StatusForbidden)
		return
	}
	if !verifyPinProof(currentPIN, req.Challenge, req.PinProof) {
		http.Error(w, "invalid PIN proof", http.StatusForbidden)
		return
	}

	peerIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		peerIP = host
	}

	offerID, err := newTransferID()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	po := &pendingOffer{
		ID:          offerID,
		TransferID:  req.TransferID,
		SenderName:  req.SenderName,
		SenderEmail: req.SenderEmail,
		Filename:    req.Filename,
		Size:        req.Size,
		PeerIP:      peerIP,
		PIN:         currentPIN,
		Salt:        append([]byte(nil), currentSalt...),
		CreatedAt:   time.Now(),
		resultCh:    make(chan offerDecision, 1),
	}

	offersMu.Lock()
	pendingOffers[offerID] = po
	offersMu.Unlock()

	if appContext != nil {
		runtime.EventsEmit(appContext, "incoming-offer", IncomingOfferEvent{
			ID:          offerID,
			TransferID:  req.TransferID,
			SenderName:  req.SenderName,
			SenderEmail: req.SenderEmail,
			Filename:    req.Filename,
			Size:        req.Size,
			SizeLabel:   byteCountDecimal(req.Size),
			PeerIP:      peerIP,
			Tip:         verifySenderTip,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"status":"pending","offer_id":"` + offerID + `"}`))
}

// RespondToOffer is called from the UI after Accept/Reject.
func RespondToOffer(offerID string, accept bool) string {
	offersMu.Lock()
	po, ok := pendingOffers[offerID]
	offersMu.Unlock()
	if !ok {
		return "Error: offer not found or already handled"
	}

	if !accept {
		select {
		case po.resultCh <- offerDecision{accept: false}:
		default:
		}
		_ = AppendAudit(AuditEntry{
			Direction: "inbound",
			PeerIP:    po.PeerIP,
			Name:      po.SenderName,
			Email:     po.SenderEmail,
			Filename:  po.Filename,
			Size:      po.Size,
			Result:    "rejected",
		})
		rotatePIN()
		if appContext != nil {
			runtime.EventsEmit(appContext, "pin-updated", currentPIN)
		}
		return "Rejected"
	}

	token, err := newUploadToken()
	if err != nil {
		select {
		case po.resultCh <- offerDecision{accept: false}:
		default:
		}
		return "Error: could not mint token"
	}

	tokensMu.Lock()
	uploadTokens[token] = &uploadGrant{
		TransferID:  po.TransferID,
		SenderName:  po.SenderName,
		SenderEmail: po.SenderEmail,
		Filename:    po.Filename,
		Size:        po.Size,
		PeerIP:      po.PeerIP,
		PIN:         po.PIN,
		Salt:        append([]byte(nil), po.Salt...),
		Expires:     time.Now().Add(10 * time.Minute),
	}
	tokensMu.Unlock()

	select {
	case po.resultCh <- offerDecision{accept: true, token: token}:
	default:
	}

	_ = AppendAudit(AuditEntry{
		Direction: "inbound",
		PeerIP:    po.PeerIP,
		Name:      po.SenderName,
		Email:     po.SenderEmail,
		Filename:  po.Filename,
		Size:      po.Size,
		Result:    "accepted",
	})
	return "Accepted"
}

// WaitOfferDecision blocks until UI responds or timeout (used by sender via HTTP wait endpoint).
func waitOfferDecisionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	offerID := r.URL.Query().Get("id")
	if offerID == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	offersMu.Lock()
	po, ok := pendingOffers[offerID]
	offersMu.Unlock()
	if !ok {
		// may already be decided — check is hard; return gone
		http.Error(w, "offer not found", http.StatusNotFound)
		return
	}

	select {
	case dec := <-po.resultCh:
		offersMu.Lock()
		delete(pendingOffers, offerID)
		offersMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if !dec.accept {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"status":"rejected"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(offerAccepted{Status: "accepted", Token: dec.token})
	case <-time.After(2 * time.Minute):
		offersMu.Lock()
		delete(pendingOffers, offerID)
		offersMu.Unlock()
		_ = AppendAudit(AuditEntry{
			Direction: "inbound",
			PeerIP:    po.PeerIP,
			Name:      po.SenderName,
			Email:     po.SenderEmail,
			Filename:  po.Filename,
			Size:      po.Size,
			Result:    "failed",
			Detail:    "offer timeout",
		})
		rotatePIN()
		if appContext != nil {
			runtime.EventsEmit(appContext, "pin-updated", currentPIN)
		}
		http.Error(w, "offer timeout", http.StatusGatewayTimeout)
	case <-r.Context().Done():
		return
	}
}

func looksLikeEmail(s string) bool {
	at := strings.IndexByte(s, '@')
	if at < 1 || at >= len(s)-3 {
		return false
	}
	dot := strings.LastIndexByte(s, '.')
	return dot > at+1 && dot < len(s)-1
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	token := r.Header.Get(HeaderUploadToken)
	transferID := r.Header.Get(HeaderTransferID)
	if token == "" || transferID == "" {
		http.Error(w, "missing token", http.StatusForbidden)
		return
	}

	tokensMu.Lock()
	grant, ok := uploadTokens[token]
	if ok {
		delete(uploadTokens, token)
	}
	tokensMu.Unlock()
	if !ok || time.Now().After(grant.Expires) || grant.TransferID != transferID {
		http.Error(w, "invalid or expired token", http.StatusForbidden)
		return
	}

	key, err := deriveSessionKey(grant.PIN, grant.Salt, grant.TransferID)
	if err != nil {
		http.Error(w, "key derivation failed", http.StatusInternalServerError)
		return
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	downloadPath := filepath.Join(homeDir, "Downloads", grant.Filename)
	dst, err := os.Create(downloadPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	var lastPerc int
	onProg := func(n int64) {
		if grant.Size > 0 {
			percent := int(float64(n) / float64(grant.Size) * 100)
			if percent > lastPerc {
				lastPerc = percent
				if appContext != nil {
					runtime.EventsEmit(appContext, "download-progress", percent)
				}
			}
		}
	}

	written, err := decryptStream(dst, r.Body, key, grant.Size, onProg)
	if err != nil {
		_ = os.Remove(downloadPath)
		_ = AppendAudit(AuditEntry{
			Direction: "inbound",
			PeerIP:    grant.PeerIP,
			Name:      grant.SenderName,
			Email:     grant.SenderEmail,
			Filename:  grant.Filename,
			Size:      grant.Size,
			Result:    "failed",
			Detail:    err.Error(),
		})
		http.Error(w, "decrypt failed", http.StatusBadRequest)
		rotatePIN()
		if appContext != nil {
			runtime.EventsEmit(appContext, "pin-updated", currentPIN)
		}
		return
	}

	if appContext != nil {
		runtime.EventsEmit(appContext, "download-progress", 100)
		runtime.EventsEmit(appContext, "file-received", map[string]string{
			"filename": grant.Filename,
			"sender":   grant.SenderName + " <" + grant.SenderEmail + ">",
			"path":     downloadPath,
			"size":     byteCountDecimal(written),
		})
	}

	rotatePIN()
	if appContext != nil {
		runtime.EventsEmit(appContext, "pin-updated", currentPIN)
	}

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func byteCountDecimal(b int64) string {
	const unit = 1000
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "kMGTPE"[exp])
}

func transferBaseURL(host string, port int) string {
	ip := net.ParseIP(host)
	if ip != nil && ip.To4() == nil {
		return fmt.Sprintf("https://[%s]:%d", host, port)
	}
	return fmt.Sprintf("https://%s:%d", host, port)
}

func ParseTransferTarget(address string) (host string, port int, err error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return "", 0, fmt.Errorf("empty address")
	}
	if !strings.Contains(address, ":") {
		return address, FileTransferPort, nil
	}
	h, pStr, splitErr := net.SplitHostPort(address)
	if splitErr != nil {
		return address, FileTransferPort, nil
	}
	p, convErr := strconv.Atoi(pStr)
	if convErr != nil || p <= 0 || p > 65535 {
		return "", 0, fmt.Errorf("invalid port")
	}
	return h, p, nil
}

func httpsClient(fingerprint string) *http.Client {
	return &http.Client{
		Timeout: 0,
		Transport: &http.Transport{
			TLSClientConfig: tlsClientConfig(fingerprint),
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		},
	}
}

func SendFileToPeer(address, filePath, pin, senderName, senderEmail string) error {
	senderName = strings.TrimSpace(senderName)
	senderEmail = strings.TrimSpace(strings.ToLower(senderEmail))
	if senderName == "" || !looksLikeEmail(senderEmail) {
		return fmt.Errorf("name and organizational email are required")
	}
	if len(pin) != 6 {
		return fmt.Errorf("PIN must be 6 digits")
	}

	host, port, err := ParseTransferTarget(address)
	if err != nil {
		return err
	}
	base := transferBaseURL(host, port)

	// Challenge (fingerprint optional until we know it)
	client := httpsClient("")
	chalResp, err := client.Get(base + "/localbeam/challenge")
	if err != nil {
		return fmt.Errorf("challenge: %w", err)
	}
	defer chalResp.Body.Close()
	if chalResp.StatusCode != http.StatusOK {
		return fmt.Errorf("challenge failed: %s", chalResp.Status)
	}
	var chal challengeResponse
	if err := json.NewDecoder(chalResp.Body).Decode(&chal); err != nil {
		return err
	}
	salt, err := b64Decode(chal.Salt)
	if err != nil {
		return fmt.Errorf("bad salt")
	}

	client = httpsClient(chal.Fingerprint)

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()
	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}
	fileSize := fileInfo.Size()
	filename := filepath.Base(filePath)

	transferID, err := newTransferID()
	if err != nil {
		return err
	}

	offerBody, _ := json.Marshal(offerRequest{
		TransferID:  transferID,
		Challenge:   chal.Nonce,
		PinProof:    pinProof(pin, chal.Nonce),
		SenderName:  senderName,
		SenderEmail: senderEmail,
		Filename:    filename,
		Size:        fileSize,
	})

	offerHTTP, err := client.Post(base+"/offer", "application/json", bytes.NewReader(offerBody))
	if err != nil {
		return fmt.Errorf("offer: %w", err)
	}
	defer offerHTTP.Body.Close()

	if offerHTTP.StatusCode == http.StatusForbidden {
		return fmt.Errorf("wrong PIN")
	}
	if offerHTTP.StatusCode != http.StatusAccepted && offerHTTP.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(offerHTTP.Body, 2048))
		return fmt.Errorf("offer failed: %s", strings.TrimSpace(string(body)))
	}

	var offerPending struct {
		Status  string `json:"status"`
		OfferID string `json:"offer_id"`
	}
	if err := json.NewDecoder(offerHTTP.Body).Decode(&offerPending); err != nil {
		return err
	}
	if offerPending.OfferID == "" {
		return fmt.Errorf("missing offer id")
	}

	if appContext != nil {
		runtime.EventsEmit(appContext, "waiting-accept", map[string]string{
			"peer":     address,
			"filename": filename,
		})
	}

	// Poll wait endpoint
	waitURL := fmt.Sprintf("%s/offer/wait?id=%s", base, offerPending.OfferID)
	waitClient := httpsClient(chal.Fingerprint)
	waitClient.Timeout = 130 * time.Second
	waitResp, err := waitClient.Get(waitURL)
	if err != nil {
		return fmt.Errorf("waiting for accept: %w", err)
	}
	defer waitResp.Body.Close()
	if waitResp.StatusCode == http.StatusForbidden {
		_ = AppendAudit(AuditEntry{
			Direction: "outbound",
			PeerIP:    host,
			Name:      senderName,
			Email:     senderEmail,
			Filename:  filename,
			Size:      fileSize,
			Result:    "rejected",
		})
		return fmt.Errorf("receiver rejected the transfer")
	}
	if waitResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(waitResp.Body, 2048))
		return fmt.Errorf("accept wait failed: %s", strings.TrimSpace(string(body)))
	}
	var accepted offerAccepted
	if err := json.NewDecoder(waitResp.Body).Decode(&accepted); err != nil {
		return err
	}
	if accepted.Token == "" {
		return fmt.Errorf("missing upload token")
	}

	key, err := deriveSessionKey(pin, salt, transferID)
	if err != nil {
		return err
	}

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		var lastPerc int
		_, err := encryptStream(pw, file, key, func(n int64) {
			if fileSize > 0 && appContext != nil {
				percent := int(float64(n) / float64(fileSize) * 100)
				if percent > lastPerc {
					lastPerc = percent
					runtime.EventsEmit(appContext, "upload-progress", percent)
				}
			}
		})
		if err != nil {
			_ = pw.CloseWithError(err)
		}
	}()

	upReq, err := http.NewRequest(http.MethodPost, base+"/upload", pr)
	if err != nil {
		return err
	}
	upReq.Header.Set(HeaderUploadToken, accepted.Token)
	upReq.Header.Set(HeaderTransferID, transferID)
	upReq.Header.Set(HeaderFileSize, fmt.Sprintf("%d", fileSize))
	upReq.Header.Set(HeaderLocalBeamVer, ProtocolVersion)
	upReq.Header.Set(HeaderSenderName, senderName)
	upReq.Header.Set(HeaderSenderEmail, senderEmail)
	upReq.Header.Set("Content-Type", "application/octet-stream")

	upResp, err := client.Do(upReq)
	if err != nil {
		return err
	}
	defer upResp.Body.Close()
	if upResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(upResp.Body, 2048))
		_ = AppendAudit(AuditEntry{
			Direction: "outbound",
			PeerIP:    host,
			Name:      senderName,
			Email:     senderEmail,
			Filename:  filename,
			Size:      fileSize,
			Result:    "failed",
			Detail:    strings.TrimSpace(string(body)),
		})
		return fmt.Errorf("upload failed: %s", strings.TrimSpace(string(body)))
	}

	if appContext != nil {
		runtime.EventsEmit(appContext, "upload-progress", 100)
	}
	_ = AppendAudit(AuditEntry{
		Direction: "outbound",
		PeerIP:    host,
		Name:      senderName,
		Email:     senderEmail,
		Filename:  filename,
		Size:      fileSize,
		Result:    "sent",
	})
	return nil
}
