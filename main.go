package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"golang.org/x/net/proxy"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Targets []string `yaml:"targets"`
}

func TakeScreenshot(url string, filePath string) error {
	// 1. Chrome'u Tor Proxy ile yapılandır
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ProxyServer("socks5://127.0.0.1:9050"), // Tor portu
		chromedp.Flag("headless", false),                // Arayüzsüz çalıştır
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	// 2. Context oluştur ve Timeout ekle (Tor yavaş olduğu için 90 saniye ideal)
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	var buf []byte
	// 3. Aksiyonları çalıştır
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible(`body`, chromedp.ByQuery), // Sayfanın yüklenmesini bekle
		chromedp.CaptureScreenshot(&buf),
	)
	if err != nil {
		return err
	}

	// 4. Dosyaya kaydet
	return os.WriteFile(filePath, buf, 0o644)
}
func GetTorClient() (*http.Client, error) {
	// Connection endpoint
	torproxy := "127.0.0.1:9050"

	// Creating dialer object
	dialer, err := proxy.SOCKS5("tcp", torproxy, nil, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("Proxy dialer error: %w", err)
	}

	// Create HTTP transport
	transport := &http.Transport{
		Dial:                dialer.Dial,
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	// Create new client and return that object
	client := &http.Client{
		Transport: transport,
		Timeout:   time.Second * 60,
	}
	return client, nil
}

func TorConnectivityCheck(client *http.Client, site string) {
	log.Println("Checking IP leak via Tor network...")
	resp, err := client.Get(site)
	if err != nil {
		log.Println("Are you connected to Tor network?")
		log.Fatalf("Request error: %v", err)
	}
	// Read body of response
	body, _ := io.ReadAll(resp.Body)
	log.Printf("IP address: %s\n", body)
}

func ReadTargets(filename string) ([]string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("file can not readed: %v", err)
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("YAML can not readed: %v", err)
	}
	return config.Targets, nil
}

func GetHTMLBody(client *http.Client, target string) (*http.Response, error) {
	// Create request manually
	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		return nil, fmt.Errorf("Request can not created: %v", err)
	}
	// Set User-Agent header
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	// Send request, get response
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK { // 200 değilse
		resp.Body.Close()
		return nil, fmt.Errorf("Server denied to response, status code: %d", resp.StatusCode)
	}
	return resp, nil
}

func WriteHTMLContent(content io.ReadCloser, filename string) error {
	body, _ := io.ReadAll(content)
	err := os.WriteFile(filename, body, 0o644)
	if err != nil {
		return err
	}
	return nil
}

func GetFilePath(target string) string {
	filename := strings.TrimSuffix(target, "/")
	filename = strings.Replace(filename, "://", ".", -1)
	filename = strings.ReplaceAll(filename, "/", "_")
	return filename
}

func ConcatExtension(filename, extension string) string {
	return filename + extension
}

// Bu fonksiyonu şu şekilde güncelle:
func SaveScanLog(logFile string, logMessage string) error {
	f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(time.Now().Format("2006-01-02 15:04:05") + " " + logMessage + "\n")
	return err
}

func DirExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false // Klasör yok
	}
	// Hata yoksa ve bu bir dizinse (IsDir) true döner
	return err == nil && info.IsDir()
}

func main() {
	client, err := GetTorClient()
	if err != nil {
		log.Fatalf("can not created tor client: %v", err)
	}
	TorConnectivityCheck(client, "http://check.torproject.org/api/ip")
	targets, err := ReadTargets("targets.yaml")
	if err != nil {
		log.Fatal(err)
	}
	dirName := time.Now().Format("2006-01-02")
	if !DirExists(dirName) {
		err = os.Mkdir(dirName, 0755)
		if err != nil {
			log.Fatalf("[ERR] can not create the directory: %v", err)
		}
	}
	// Main loop for targets
	for _, target := range targets {
		// Trim whitespaces ' '
		target = strings.TrimSpace(target)
		// Connect to target
		resp, err := GetHTMLBody(client, target)
		if err != nil {
			message := fmt.Sprintf("[ERR] Scanning: %v -> %v", target, err)
			log.Println(message)
			err = SaveScanLog("scan_report.log", message)
			if err != nil {
				log.Printf("Can not write to log file: %v", err)
			}
			continue
		} else {
			message := fmt.Sprintf("[INFO] Scanning: %v -> SUCCESS", target)
			log.Println(message)
			err = SaveScanLog("scan_report.log", message)
			if err != nil {
				log.Printf("Could not write to log file: %v", err)
			}
		}
		// Save the content by date
		htmlFileName := dirName + "/" + ConcatExtension(GetFilePath(target), ".html")
		// Write HTML response body to file
		err = WriteHTMLContent(resp.Body, htmlFileName)
		resp.Body.Close()
		if err != nil {
			log.Printf("HTML content could not be saved: %v: %v", target, err)
			continue
		}
		// Take screenshot
		pngFileName := dirName + "/" + ConcatExtension(GetFilePath(target), ".png")

		err = TakeScreenshot(target, pngFileName)
		if err != nil {
			message := fmt.Sprintf("[ERR] Screenshot could not be taken (%s): %v", target, err)
			log.Println(message)
			err = SaveScanLog("scan_report.log", message)
			if err != nil {
				log.Printf("Could not write to log file: %v", err)
			}
		}
	}
}
