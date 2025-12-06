package main

import (
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const fileTransferPort = 34567

var appContext context.Context
var currentPIN string

// --- ProgressReader (בשביל השולח) ---
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
			runtime.EventsEmit(pr.Context, "upload-progress", percent)
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
	fmt.Printf("🔒 Security PIN generated: %s\n", currentPIN)

	http.HandleFunc("/upload", uploadHandler)
	fmt.Printf("File Server listening on port %d...\n", fileTransferPort)
	http.ListenAndServe(fmt.Sprintf(":%d", fileTransferPort), nil)
}

// --- ה-Handler החדש והחכם לקבלת קבצים ---
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	// 1. בדיקת PIN
	incomingPin := r.Header.Get("X-PIN")
	if incomingPin != currentPIN {
		http.Error(w, "Invalid PIN", http.StatusForbidden)
		return
	}

	// 2. קבלת גודל הקובץ (שהשולח שלח לנו)
	fileSizeStr := r.Header.Get("X-File-Size")
	totalSize, _ := strconv.ParseInt(fileSizeStr, 10, 64)

	fmt.Println("Incoming file...", "Size:", totalSize)

	// 3. שימוש ב-MultipartReader לקריאה זורמת (Streaming)
	reader, err := r.MultipartReader()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// לולאה שעוברת על החלקים בבקשה (אנחנו מצפים לחלק אחד בשם "file")
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println("Error reading part:", err)
			break
		}

		if part.FormName() == "file" {
			// יצירת הקובץ בדיסק
			homeDir, _ := os.UserHomeDir()
			downloadPath := filepath.Join(homeDir, "Downloads", part.FileName())
			dst, err := os.Create(downloadPath)
			if err != nil {
				return
			}
			defer dst.Close()

			// --- לולאת ההעתקה עם דיווח התקדמות ---
			buf := make([]byte, 32*1024) // Buffer של 32KB
			var current int64
			var lastPerc int

			for {
				n, err := part.Read(buf)
				if n > 0 {
					// כתיבה לדיסק
					dst.Write(buf[:n])
					current += int64(n)

					// חישוב אחוזים ודיווח למסך
					if totalSize > 0 {
						percent := int(float64(current) / float64(totalSize) * 100)
						// מעדכנים רק כשהאחוז משתנה
						if percent > lastPerc {
							lastPerc = percent
							if appContext != nil {
								runtime.EventsEmit(appContext, "download-progress", percent)
							}
						}
					}
				}
				if err == io.EOF {
					break
				}
				if err != nil {
					fmt.Println("Error copying:", err)
					break
				}
			}

			fmt.Printf("File saved: %s\n", downloadPath)
			
			// דיווח סופי למסך (ההודעה הירוקה)
			if appContext != nil {
				info := map[string]string{
					"filename": part.FileName(),
					"sender":   r.RemoteAddr,
					"path":     downloadPath,
					"size":     byteCountDecimal(totalSize),
				}
				runtime.EventsEmit(appContext, "file-received", info)
			}
		}
	}

	w.Write([]byte("File uploaded successfully"))
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

func SendFileToPeer(ip string, filePath string, pin string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	fileInfo, _ := file.Stat()
	fileSize := fileInfo.Size()

	bodyReader, bodyWriter := io.Pipe()
	multipartWriter := multipart.NewWriter(bodyWriter)

	fileReaderWithProgress := &ProgressReader{
		Reader:  file,
		Total:   fileSize,
		Context: appContext,
	}

	go func() {
		defer bodyWriter.Close()
		part, _ := multipartWriter.CreateFormFile("file", filepath.Base(filePath))
		io.Copy(part, fileReaderWithProgress)
		multipartWriter.Close()
	}()

	targetUrl := fmt.Sprintf("http://%s:%d/upload", ip, fileTransferPort)
	req, _ := http.NewRequest("POST", targetUrl, bodyReader)
	req.ContentLength = fileSize // עוזר לפרוטוקול
	req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	req.Header.Set("X-PIN", pin)
	// --- הוספנו את הכותרת הזו כדי שהמקבל ידע את הגודל מראש ---
	req.Header.Set("X-File-Size", fmt.Sprintf("%d", fileSize))

	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("wrong PIN code")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server error: %s", resp.Status)
	}

	return nil
}