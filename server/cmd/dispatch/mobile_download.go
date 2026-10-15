package main

import (
	"bytes"
	"fmt"
	"html/template"
	"mime"
	"net/http"
	"os"
	"path/filepath"
)

// Only a single operator-selected APK is exposed, never its containing directory.
func mobileDownloadHandler(apkPath string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/download/" && r.URL.Path != "/download/client.apk" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var info os.FileInfo
		// Check before opening so an accidentally configured FIFO/device cannot block.
		info, err := os.Stat(apkPath)
		var file *os.File
		if err == nil && !info.Mode().IsRegular() {
			err = fmt.Errorf("APK is not a regular file")
		}
		if err == nil {
			file, err = os.Open(apkPath)
		}
		if err == nil {
			defer file.Close()
			info, err = file.Stat()
			if err == nil && !info.Mode().IsRegular() {
				err = fmt.Errorf("APK is not a regular file")
			}
		}
		available := err == nil
		if r.URL.Path == "/download/client.apk" {
			if !available {
				http.Error(w, "Android download is not available yet", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "application/vnd.android.package-archive")
			w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filepath.Base(apkPath)}))
			http.ServeContent(w, r, filepath.Base(apkPath), info.ModTime(), file)
			return
		}
		data := struct {
			Available      bool
			Filename, Size string
		}{Available: available}
		if available {
			data.Filename = filepath.Base(apkPath)
			data.Size = fmt.Sprintf("%.1f MiB (%d bytes)", float64(info.Size())/(1024*1024), info.Size())
		}
		var page bytes.Buffer
		if err := mobileDownloadPage.Execute(&page, data); err != nil {
			http.Error(w, "download page unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
		w.Header().Set("Content-Length", fmt.Sprint(page.Len()))
		if r.Method == http.MethodGet {
			w.Write(page.Bytes())
		}
	})
}

var mobileDownloadPage = template.Must(template.New("download").Parse(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>幻想 · 安卓客户端下载</title>
<style>*{box-sizing:border-box}body{margin:0;background:#101b24;color:#eee8da;font:16px/1.7 system-ui,-apple-system,sans-serif;padding:48px 22px}main{max-width:520px;margin:auto}small{color:#c6ac72;letter-spacing:3px}h1{font-size:32px;margin:14px 0}p{color:#b8c2c9}.card{margin-top:30px;padding:24px;border:1px solid #43515b;border-radius:18px;background:#192832}.button{display:block;padding:15px;text-align:center;background:#e4c68a;color:#18212a;text-decoration:none;border-radius:10px;font-weight:700;font-size:19px}.facts{font-size:14px;color:#c7cdd0;overflow-wrap:anywhere}footer{font-size:13px;margin-top:24px;color:#9aa8b1}</style></head>
<body><main><small>幻想 · 移动端</small><h1>把幻想装进口袋</h1><p>安卓客户端测试版</p><section class="card">
{{if .Available}}<a class="button" href="/download/client.apk" download>下载安卓安装包</a><p class="facts">{{.Filename}}<br>{{.Size}}</p><p class="facts">安装包与游戏资源分别配置。请按客户端的资源配置说明操作。</p>
{{else}}<h2>安装包暂未就绪</h2><p>请等待服务端配置安卓安装包后刷新此页。</p>{{end}}
</section><footer>请让手机与电脑处于同一局域网。下载完成后，在安卓手机上打开 APK 安装。仅安装你信任的来源提供的文件。</footer></main></body></html>`))
