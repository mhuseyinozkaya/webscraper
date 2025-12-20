package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

func main() {
	var validurls []string /* Gezilecek geçerli URL'lerin listesi */
	rawurls := os.Args[1:]

	for _, url := range rawurls {
		if IsValidURL(url) {
			validurls = append(validurls, url)
		}
	}

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("enable-automation", false),
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
		chromedp.WindowSize(1920, 1080),
	)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelAlloc()

	for _, url := range validurls {
		ctx, cancel := chromedp.NewContext(allocCtx)
		defer cancel()

		tCtx, tCancel := context.WithTimeout(ctx, 30*time.Second)
		var buf []byte         /* Ekran görüntüsünün kaydedileceği buffer */
		var htmlContent string /* HTML içeriğinin kaydedileceği buffer */
		var nodes []*cdp.Node  /* Bağlantıları kaydetmek için düğüm yapısı */

		// Web sitesinde yapılacak işlemler
		chromedp_DoList := chromedp.Tasks{
			chromedp.WaitReady("body"),
			chromedp.Sleep(2000 * time.Millisecond),
			chromedp.OuterHTML("html", &htmlContent),
			chromedp.Nodes("[href]", &nodes), // Bütün href niteliklerini node'a aktar
			TakeScreenshot(100, &buf),
		}

		if err := chromedp.Run(tCtx, chromedp.Navigate(url), chromedp_DoList); err != nil {
			log.Printf("Hata veya Zaman Aşımı (%s): %v\n", url, err)
			tCancel()
			continue
		}
		tCancel()

		var links []string
		// Sayfanın içindeki bağlantıları kaydetme
		for _, n := range nodes {
			href := n.AttributeValue("href")
			href = strings.TrimSpace(href)
			// Geçersiz olan href bağlantılarını atlama
			if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "javascript:") {
				continue
			}
			// domain + path yaparak geçerli bağlantı oluşturma
			fixed := ConcatURL(url, href)
			if fixed == "" {
				continue
			}
			links = append(links, fixed)
		}
		// Bağlantıları kaydetme
		SaveLinksToFile(ParseFilePath(url, ".txt"), links)

		// HTML içeriğini kaydetme
		filename := ParseFilePath(url, ".html")
		htmlbuf := []byte(htmlContent)
		if WriteSavedContent(filename, &htmlbuf) {
			fmt.Printf("HTML içeriği kaydedildi: %s\n", filename)
		} else {
			fmt.Printf("HTML içeriği kaydedilemedi: %s\n", url)
		}

		// Ekran görüntüsünü kaydetme
		imagename := ParseFilePath(url, ".png")
		if WriteSavedContent(imagename, &buf) {
			fmt.Printf("Ekran görüntüsü kaydedildi: %s\n", imagename)
		} else {
			fmt.Printf("Ekran görüntüsü alınamadı: %s\n", url)
		}
	}
}

func SaveLinksToFile(filename string, links []string) {
	file, err := os.Create(filename)
	if err != nil {
		log.Printf("Dosya oluşturulamadı: %v", err)
		return
	}

	defer file.Close()

	for _, link := range links {
		// Linki yaz ve sonuna alt satıra geçme karakteri (\n) ekle
		_, err := file.WriteString(link + "\n")
		if err != nil {
			log.Printf("Yazma hatası: %v", err)
		}
	}
	fmt.Printf("%d adet bağlantı kaydedildi: %s\n", len(links), filename)
}

func ConcatURL(link string, href string) string {
	base, _ := url.Parse(link)
	path, err := url.Parse(href)
	if err != nil {
		return ""
	}
	return base.ResolveReference(path).String()
}
func ParseFilePath(url string, extension string) string {
	filename := strings.ReplaceAll(url, "://", ".")
	filename = strings.ReplaceAll(filename, ":", ".")
	filename = strings.Trim(filename, "/")
	filename = strings.ReplaceAll(filename, "/", ".")
	return filename + extension
}

func IsValidURL(rawurl string) bool {
	// URL'yi otomatik olarak parçalar
	u, err := url.Parse(rawurl)

	if err != nil {
		fmt.Printf("URL ayrıştırılamadı: %s\n", rawurl)
		return false
	}

	if u.Scheme == "" || u.Host == "" {
		fmt.Printf("Protokol veya host belirtilmedi: %s\n", rawurl)
		return false
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		fmt.Printf("Belirtilen protokol HTTP değil: %s\n", rawurl)
		return false
	}
	return true
}

func TakeScreenshot(quality int8, resource *[]byte) chromedp.Tasks {
	return chromedp.Tasks{
		chromedp.FullScreenshot(resource, int(quality)),
	}
}

func WriteSavedContent(file string, resource *[]byte) bool {
	if err := os.WriteFile(file, *resource, 0o644); err != nil {
		log.Println(err)
		return false
	}
	return true
}
