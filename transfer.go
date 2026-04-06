package main

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
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
	fileServerMu   sync.Mutex
	fileHTTPServer *http.Server
)

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

func generatePIN() string {
	return fmt.Sprintf("%04d", time.Now().UnixNano()%10000)
}

func StartFileServer(ctx context.Context) {
	appContext = ctx
	currentPIN = generatePIN()
	fmt.Printf("Security PIN: %s\n", currentPIN)

	mux := http.NewServeMux()
	mux.HandleFunc("/upload", uploadHandler)
	mux.HandleFunc("/localbeam/ping", pingHandler)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", FileTransferPort),
		Handler:           mux,
		ReadHeaderTimeout: 60 * time.Second,
	}

	fileServerMu.Lock()
	fileHTTPServer = srv
	fileServerMu.Unlock()

	go func() {
		fmt.Printf("File server listening on port %d\n", FileTransferPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"ok":true,"version":"%s","port":%d}`, ProtocolVersion, FileTransferPort)
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	incomingPin := r.Header.Get(HeaderPIN)
	if incomingPin != currentPIN {
		http.Error(w, "Invalid PIN", http.StatusForbidden)
		return
	}

	totalSize, _ := strconv.ParseInt(r.Header.Get(HeaderFileSize), 10, 64)

	reader, err := r.MultipartReader()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if part.FormName() != "file" {
			continue
		}
		if err := saveUploadedFile(r, part, totalSize); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		return
	}

	http.Error(w, "no file in request", http.StatusBadRequest)
}

func saveUploadedFile(r *http.Request, part *multipart.Part, totalSize int64) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	downloadPath := filepath.Join(homeDir, "Downloads", filepath.Base(part.FileName()))
	dst, err := os.Create(downloadPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	buf := make([]byte, 32*1024)
	var current int64
	var lastPerc int

	for {
		n, readErr := part.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return werr
			}
			current += int64(n)
			if totalSize > 0 {
				percent := int(float64(current) / float64(totalSize) * 100)
				if percent > lastPerc {
					lastPerc = percent
					if appContext != nil {
						runtime.EventsEmit(appContext, "download-progress", percent)
					}
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}

	if appContext != nil {
		runtime.EventsEmit(appContext, "download-progress", 100)
		info := map[string]string{
			"filename": filepath.Base(part.FileName()),
			"sender":   r.RemoteAddr,
			"path":     downloadPath,
			"size":     byteCountDecimal(totalSize),
		}
		runtime.EventsEmit(appContext, "file-received", info)
	}

	fmt.Printf("File saved: %s\n", downloadPath)
	return nil
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

func GetCurrentPIN() string {
	return currentPIN
}

// ParseTransferTarget accepts "host" or "host:port".
func transferBaseURL(host string, port int) string {
	ip := net.ParseIP(host)
	if ip != nil && ip.To4() == nil {
		return fmt.Sprintf("http://[%s]:%d", host, port)
	}
	return fmt.Sprintf("http://%s:%d", host, port)
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

func SendFileToPeer(address string, filePath string, pin string) error {
	host, port, err := ParseTransferTarget(address)
	if err != nil {
		return err
	}

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

	bodyReader, bodyWriter := io.Pipe()
	multipartWriter := multipart.NewWriter(bodyWriter)

	progressReader := &ProgressReader{
		Reader:  file,
		Total:   fileSize,
		Context: appContext,
	}

	go func() {
		defer bodyWriter.Close()
		part, werr := multipartWriter.CreateFormFile("file", filepath.Base(filePath))
		if werr != nil {
			return
		}
		_, _ = io.Copy(part, progressReader)
		_ = multipartWriter.Close()
	}()

	targetURL := transferBaseURL(host, port) + "/upload"
	req, err := http.NewRequest(http.MethodPost, targetURL, bodyReader)
	if err != nil {
		return err
	}
	req.ContentLength = -1
	req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	req.Header.Set(HeaderPIN, pin)
	req.Header.Set(HeaderFileSize, fmt.Sprintf("%d", fileSize))
	req.Header.Set(HeaderLocalBeamVer, ProtocolVersion)

	client := &http.Client{
		Timeout: 0,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("wrong PIN")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = resp.Status
		}
		return fmt.Errorf("transfer failed: %s", msg)
	}

	if appContext != nil {
		runtime.EventsEmit(appContext, "upload-progress", 100)
	}
	return nil
}
