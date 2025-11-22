package main

import (
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	baseDir  string
	tplIndex *template.Template
)

const (
	defaultPort = "5000"
	uploadDir   = "shared_files"
)

const indexHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8" />
  <title>局域网文件分享</title>
  <style>
    * { box-sizing: border-box; }
    body { margin: 0; padding: 40px 16px; background: linear-gradient(135deg,#eef2ff,#f8fbff); font-family: "Segoe UI","Helvetica Neue",Arial,sans-serif; color: #2d3436; }
    .container { max-width: 760px; margin: 0 auto; background: #fff; border-radius: 18px; box-shadow: 0 18px 45px rgba(93,118,247,0.15); padding: 32px; }
    header { display: flex; flex-direction: column; gap: 8px; margin-bottom: 24px; }
    header h1 { font-size: 28px; margin: 0; color: #3c40c6; }
    header p { margin: 0; color: #636e72; }
    .info-bar { display: grid; gap: 16px; grid-template-columns: repeat(auto-fit,minmax(220px,1fr)); margin-bottom: 24px; }
    .info-card { padding: 16px; border-radius: 12px; background: #f5f7ff; border: 1px solid rgba(93,118,247,0.15); }
    .info-card span { display: block; font-size: 12px; letter-spacing: 0.08em; text-transform: uppercase; color: #636e72; margin-bottom: 4px; }
    .info-card strong, .info-card a { font-size: 16px; color: #2d3436; text-decoration: none; word-break: break-all; }
    form { display: flex; gap: 12px; align-items: center; padding: 20px; border-radius: 14px; background: #f9fbff; border: 1px dashed rgba(93,118,247,0.35); margin-bottom: 28px; }
    input[type="file"] { flex: 1; }
    button { background: linear-gradient(135deg,#5d76f7,#6c5ce7); color: #fff; border: none; padding: 10px 22px; border-radius: 10px; cursor: pointer; font-size: 15px; transition: transform .15s ease, box-shadow .15s ease; }
    button:hover { transform: translateY(-1px); box-shadow: 0 10px 18px rgba(93,118,247,0.25); }
    .file-list { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 12px; }
    .file-item { display: flex; justify-content: space-between; align-items: center; padding: 16px 20px; border-radius: 14px; background: #fbfbff; border: 1px solid rgba(93,118,247,0.12); box-shadow: inset 0 0 0 1px rgba(93,118,247,0.05); }
    .file-name { font-weight: 600; color: #2d3436; word-break: break-all; }
    .file-link { color: #5d76f7; text-decoration: none; font-weight: 600; }
    .file-link:hover { text-decoration: underline; }
    .empty { padding: 28px; text-align: center; color: #95a5a6; border: 1px dashed rgba(93,118,247,0.2); border-radius: 14px; background: #fdfdff; }
  </style>
</head>
<body>
  <div class="container">
    <header>
      <h1>局域网文件分享</h1>
      <p>在同一局域网的设备上访问 <strong>{{ .LanURL }}</strong> 即可下载文件</p>
    </header>
    <div class="info-bar">
      <div class="info-card">
        <span>共享目录</span>
        <strong>{{ .FolderName }}</strong>
      </div>
      <div class="info-card">
        <span>访问地址</span>
        <a href="{{ .LanURL }}" target="_blank" rel="noopener">{{ .LanURL }}</a>
      </div>
    </div>
    <form action="/upload" method="post" enctype="multipart/form-data">
      <input type="file" name="file" required />
      <button type="submit">上传文件</button>
    </form>
    <h2>可下载文件</h2>
    <ul class="file-list">
      {{- if .Files }}
        {{- range .Files }}
          <li class="file-item">
            <span class="file-name">{{ . }}</span>
            <a class="file-link" href="/files/{{ . }}">下载</a>
          </li>
        {{- end }}
      {{- else }}
        <li class="empty">暂无可用文件，先上传一个吧。</li>
      {{- end }}
    </ul>
  </div>
</body>
</html>
`

func main() {
	initPaths()
	tplIndex = template.Must(template.New("index").Parse(indexHTML))
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/upload", handleUpload)
	mux.HandleFunc("/files/", handleDownload)

	addr := ":" + defaultPort
	log.Printf("共享目录: %s", filepath.Join(baseDir, uploadDir))
	log.Printf("局域网地址: %s", "http://"+netJoin(resolveLANIP(), defaultPort))
	log.Printf("本地访问: http://localhost:%s", defaultPort)
	if err := http.ListenAndServe(addr, logRequest(mux)); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}

func initPaths() {
	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("获取程序路径失败: %v", err)
	}
	baseDir = filepath.Dir(exe)
	if err = os.MkdirAll(filepath.Join(baseDir, uploadDir), 0o755); err != nil {
		log.Fatalf("创建共享目录失败: %v", err)
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "方法不支持", http.StatusMethodNotAllowed)
		return
	}
	files, err := listFiles()
	if err != nil {
		http.Error(w, "读取文件列表失败", http.StatusInternalServerError)
		return
	}
	data := map[string]any{
		"Files":      files,
		"FolderName": uploadDir,
		"LanURL":     "http://" + netJoin(resolveLANIP(), defaultPort),
	}
	if err := tplIndex.Execute(w, data); err != nil {
		log.Printf("渲染模板失败: %v", err)
		http.Error(w, "渲染错误", http.StatusInternalServerError)
	}
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "方法不支持", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		http.Error(w, "解析表单失败", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "未选择文件", http.StatusBadRequest)
		return
	}
	defer file.Close()

	name := sanitizeName(header.Filename)
	if name == "" {
		http.Error(w, "文件名无效", http.StatusBadRequest)
		return
	}
	destPath := filepath.Join(baseDir, uploadDir, name)
	out, err := os.Create(destPath)
	if err != nil {
		http.Error(w, "保存文件失败", http.StatusInternalServerError)
		return
	}
	defer out.Close()

	if _, err = io.Copy(out, file); err != nil {
		http.Error(w, "写入文件失败", http.StatusInternalServerError)
		return
	}
	log.Printf("接收文件: %s", name)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "方法不支持", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/files/")
	name = sanitizeName(name)
	if name == "" {
		http.NotFound(w, r)
		return
	}
	filePath := filepath.Join(baseDir, uploadDir, name)
	f, err := os.Open(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	http.ServeContent(w, r, name, getModTime(f), f)
}

func listFiles() ([]string, error) {
	dir := filepath.Join(baseDir, uploadDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)
	return files, nil
}

func sanitizeName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, string(os.PathSeparator), "_")
	return name
}

func resolveLANIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	local := conn.LocalAddr().(*net.UDPAddr)
	return local.IP.String()
}

func netJoin(host, port string) string {
	if strings.Contains(host, ":") {
		return "[" + host + "]:" + port
	}
	return host + ":" + port
}

func getModTime(f *os.File) (modTime time.Time) {
	info, err := f.Stat()
	if err == nil {
		modTime = info.ModTime()
	}
	return
}

func logRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
