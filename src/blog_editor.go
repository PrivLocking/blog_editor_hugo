package main

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

var (
	homeDir     = os.Getenv("HOME")
	contentDir  = homeDir + "/myblog/content"
	staticDir   = homeDir + "/myblog/upload"
	socketPath  = "/wwwFS.out/socket.blog_editor.sock"
	triggerPath = "unix:/wwwFS.in/u92/unix.hugo_update_daemon.sock"
)

const (
	postsPerPage = 30
)

var languages []string

func main() {
	debug := flag.Bool("d", false, "Enable debug logging")
	triggerTarget := flag.String("trigger-target", triggerPath, "Target for Hugo daemon trigger (unix:/path or tcp:ip:port)")
	help := flag.Bool("h", false, "Show this help message")
	flag.Parse()

	if *help {
		flag.Usage()
		os.Exit(0)
	}

	baseDir := filepath.Join(homeDir, "myblog", "content")
	dirs, err := os.ReadDir(baseDir)
	if err == nil {
		for _, dir := range dirs {
			name := dir.Name()
			if dir.IsDir() && len(name) == 2 && isAlpha(name) {
				if name == "en" {
					languages = append([]string{"en"}, languages...)
				} else {
					languages = append(languages, name)
				}
			}
		}
	}
	if len(languages) == 0 {
		languages = []string{"en"}
	}

	log.Println("Starting blog_editor...")
	log.Printf("Content Directory: %s", contentDir)
	log.Printf("Static Directory: %s", staticDir)
	log.Printf("Languages: %v", languages)
	log.Printf("Trigger target: %s", *triggerTarget)

	os.MkdirAll(contentDir, 0755)
	os.MkdirAll(staticDir, 0755)
	os.Remove(socketPath)

	mux := http.NewServeMux()
	mux.HandleFunc("/save", makeHandler(saveHandler, *debug, *triggerTarget))
	mux.HandleFunc("/upload", makeHandler(uploadHandler, *debug, *triggerTarget))
	mux.HandleFunc("/list", makeHandler(listHandler, *debug, *triggerTarget))
	mux.HandleFunc("/posts/", makeHandler(postsHandler, *debug, *triggerTarget))
	mux.HandleFunc("/readOption", makeHandler(readOptionHandler, *debug, *triggerTarget))
	mux.HandleFunc("/", makeHandler(unknownHandler, *debug, *triggerTarget))

	server := &http.Server{Handler: mux}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("Failed to listen on socket: %v", err)
	}
	if err := os.Chmod(socketPath, 0666); err != nil {
		log.Fatalf("Failed to set socket permissions: %v", err)
	}
	defer listener.Close()

	log.Println("Server running on socket:", socketPath)
	server.Serve(listener)
}

func makeHandler(fn func(http.ResponseWriter, *http.Request, bool, string), debug bool, triggerTarget string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if debug {
			log.Printf("Request: %s %s", r.Method, r.URL.Path)
			if r.Method == http.MethodPost {
				body, _ := io.ReadAll(r.Body)
				r.Body = io.NopCloser(bytes.NewBuffer(body))
				log.Printf("Body size: %d", len(body))
			}
		}
		fn(w, r, debug, triggerTarget)
	}
}

func saveHandler(w http.ResponseWriter, r *http.Request, debug bool, triggerTarget string) {
	if r.Method != http.MethodPost {
		if debug {
			log.Println("Method not allowed: expected POST")
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var post struct {
		Path           string   `json:"path"`
		Lang           string   `json:"lang"`
		Title          string   `json:"title"`
		Date           string   `json:"date"`
		Draft          bool     `json:"draft"`
		FeaturedImage  string   `json:"featured_image"`
		Description    string   `json:"description"`
		Tags           []string `json:"tags"`
		Content        string   `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&post); err != nil {
		if debug {
			log.Printf("Invalid JSON: %v", err)
		}
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if post.Title == "" {
		if debug {
			log.Println("Title is required")
		}
		http.Error(w, "Title is required", http.StatusBadRequest)
		return
	}
	if post.Lang == "" || !isValidLang(post.Lang) {
		post.Lang = languages[0]
	}
	if post.Date == "" {
		post.Date = time.Now().Format(time.RFC3339)
	} else if _, err := time.Parse(time.RFC3339, post.Date); err != nil {
		if debug {
			log.Printf("Invalid date format: %v", err)
		}
		http.Error(w, "Invalid date format", http.StatusBadRequest)
		return
	}

	now := time.Now()
	yearMonth := now.Format("2006/01")
	if post.Path == "" {
		filename := now.Format("20060102_150405") + ".md"
		post.Path = filepath.Join(post.Lang, "post", yearMonth, filename)
	} else if !strings.HasPrefix(post.Path, post.Lang+"/post/") || strings.Contains(post.Path, "..") {
		if debug {
			log.Printf("Invalid path: %s", post.Path)
		}
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	metadata := map[string]interface{}{
		"title":          post.Title,
		"date":           post.Date,
		"draft":          post.Draft,
		"featured_image": post.FeaturedImage,
		"description":    post.Description,
		"tags":           post.Tags,
	}
	var buf bytes.Buffer
	buf.WriteString("+++\n")
	if err := toml.NewEncoder(&buf).Encode(metadata); err != nil {
		if debug {
			log.Printf("Failed to encode TOML: %v", err)
		}
		http.Error(w, "Failed to encode TOML", http.StatusInternalServerError)
		return
	}
	buf.WriteString("\n+++\n")
	buf.WriteString(post.Content)

	fullPath := filepath.Join(contentDir, post.Path)
	os.MkdirAll(filepath.Dir(fullPath), 0755)
	if err := os.WriteFile(fullPath, buf.Bytes(), 0644); err != nil {
		if debug {
			log.Printf("Failed to save file: %v", err)
		}
		http.Error(w, "Failed to save file", http.StatusInternalServerError)
		return
	}

	go triggerHugo(triggerTarget, debug)

	if debug {
		log.Printf("Saved file: %s, size: %d bytes", fullPath, buf.Len())
	}
	fmt.Fprintf(w, "Saved: %s", post.Path)
}

func triggerHugo(target string, debug bool) {
	var network, addr string
	if strings.HasPrefix(target, "unix:") {
		network = "unix"
		addr = strings.TrimPrefix(target, "unix:")
	} else if strings.HasPrefix(target, "tcp:") {
		network = "tcp"
		addr = strings.TrimPrefix(target, "tcp:")
	} else {
		return
	}

	if debug {
		log.Printf("Triggering Hugo daemon at: %s", target)
	}

	conn, err := net.DialTimeout(network, addr, 1*time.Second)
	if err != nil {
		if debug {
			log.Printf("Trigger failed: %v", err)
		}
		return
	}
	conn.Close()
	if debug {
		log.Println("Trigger sent successfully")
	}
}

func uploadHandler(w http.ResponseWriter, r *http.Request, debug bool, triggerTarget string) {
	if r.Method != http.MethodPost {
		if debug {
			log.Println("Method not allowed: expected POST")
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		if debug {
			log.Printf("Failed to get image: %v", err)
		}
		http.Error(w, "Failed to get image", http.StatusBadRequest)
		return
	}
	defer file.Close()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, file)
	if err != nil {
		if debug {
			log.Printf("Failed to read image: %v", err)
		}
		http.Error(w, "Failed to read image", http.StatusInternalServerError)
		return
	}
	content := buf.Bytes()

	hash := md5.Sum(content)
	hashStr := hex.EncodeToString(hash[:])
	ext := filepath.Ext(header.Filename)
	filename := hashStr + ext
	fullPath := filepath.Join(staticDir, filename)

	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		out, err := os.Create(fullPath)
		if err != nil {
			if debug {
				log.Printf("Failed to save image: %v", err)
			}
			http.Error(w, "Failed to save image", http.StatusInternalServerError)
			return
		}
		defer out.Close()
		_, err = io.Copy(out, bytes.NewReader(content))
		if err != nil {
			if debug {
				log.Printf("Failed to write image: %v", err)
			}
			http.Error(w, "Failed to write image", http.StatusInternalServerError)
			return
		}
	}

	if debug {
		log.Printf("Uploaded image: %s, size: %d bytes", fullPath, len(content))
	}

	response := map[string]string{
		"hash":     filename,
		"original": header.Filename,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func listHandler(w http.ResponseWriter, r *http.Request, debug bool, triggerTarget string) {
	lang := r.URL.Query().Get("lang")
	if !isValidLang(lang) {
		lang = languages[0]
	}

	now := time.Now()
	currentYearMonth := now.Format("2006/01")
	dir := filepath.Join(contentDir, lang, "post", currentYearMonth)
	var posts []string

	if debug {
		log.Printf("Trying current month: %s", dir)
	}

	files, err := os.ReadDir(dir)
	if err == nil {
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), ".md") {
				posts = append(posts, filepath.Join(lang, "post", currentYearMonth, file.Name()))
			}
		}
	}

	if debug {
		log.Printf("Found %d posts in current month", len(posts))
	}

	if len(posts) < postsPerPage {
		baseDir := filepath.Join(contentDir, lang, "post")
		currentMonth := now
		for i := 0; i < 12 && len(posts) < postsPerPage; i++ {
			currentMonth = currentMonth.AddDate(0, -1, 0)
			yearMonth := currentMonth.Format("2006/01")
			if yearMonth == currentYearMonth {
				continue
			}
			dir = filepath.Join(baseDir, yearMonth)
			files, err = os.ReadDir(dir)
			if err == nil {
				for _, file := range files {
					if !file.IsDir() && strings.HasSuffix(file.Name(), ".md") {
						posts = append(posts, filepath.Join(lang, "post", yearMonth, file.Name()))
					}
				}
			}
		}

		if debug && len(posts) > 0 {
			log.Printf("Supplementing to 30, searched %s, found %d total", baseDir, len(posts))
		}
	}

	sortPostsByFilename(posts)

	if len(posts) > postsPerPage {
		posts = posts[:postsPerPage]
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	start := (page - 1) * postsPerPage
	end := start + postsPerPage
	if start > len(posts) {
		start = len(posts)
	}
	if end > len(posts) {
		end = len(posts)
	}

	if debug {
		log.Printf("Listing posts: %d total, page %d, showing %d-%d, lang: %s", len(posts), page, start+1, end, lang)
	}

	response := map[string]interface{}{
		"posts": posts[start:end],
		"total": len(posts),
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func sortPostsByFilename(posts []string) {
	sortFunc := func(i, j int) bool {
		fileI := filepath.Base(posts[i])
		fileJ := filepath.Base(posts[j])
		return fileI > fileJ
	}
	for i := 0; i < len(posts)-1; i++ {
		for j := i + 1; j < len(posts); j++ {
			if !sortFunc(i, j) {
				posts[i], posts[j] = posts[j], posts[i]
			}
		}
	}
}

func postsHandler(w http.ResponseWriter, r *http.Request, debug bool, triggerTarget string) {
	if r.Method != http.MethodGet {
		if debug {
			log.Println("Method not allowed: expected GET")
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/posts/")
	lang := r.URL.Query().Get("lang")
	if !isValidLang(lang) {
		lang = languages[0]
	}
	if !strings.HasPrefix(name, lang+"/post/") || strings.Contains(name, "..") {
		if debug {
			log.Printf("Invalid post name: %s", name)
		}
		http.Error(w, "Invalid post name", http.StatusBadRequest)
		return
	}

	fullPath := filepath.Join(contentDir, name)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		if debug {
			log.Printf("Failed to read post: %v", err)
		}
		http.Error(w, "Failed to read post", http.StatusNotFound)
		return
	}

	parts := strings.SplitN(string(content), "+++\n", 3)
	if len(parts) < 3 {
		if debug {
			log.Println("Invalid post format: missing front matter delimiters")
		}
		http.Error(w, "Invalid post format", http.StatusInternalServerError)
		return
	}
	frontMatter, body := parts[1], parts[2]

	var metadata map[string]interface{}
	if err := toml.Unmarshal([]byte(frontMatter), &metadata); err != nil {
		if debug {
			log.Printf("Failed to parse TOML: %v", err)
		}
		http.Error(w, "Failed to parse TOML", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"title":          metadata["title"],
		"date":           metadata["date"],
		"draft":          metadata["draft"],
		"featured_image": metadata["featured_image"],
		"description":    metadata["description"],
		"tags":           metadata["tags"],
		"content":        body,
	}

	if debug {
		log.Printf("Served post: %s, size: %d bytes", fullPath, len(content))
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func readOptionHandler(w http.ResponseWriter, r *http.Request, debug bool, triggerTarget string) {
	if r.Method != http.MethodGet {
		if debug {
			log.Println("Method not allowed: expected GET")
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := map[string][]string{
		"lang": languages,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func unknownHandler(w http.ResponseWriter, r *http.Request, debug bool, triggerTarget string) {
	if debug {
		body, _ := io.ReadAll(r.Body)
		log.Printf("Unknown path: %s, method: %s, body size: %d bytes", r.URL.Path, r.Method, len(body))
	}
	http.Error(w, "Not found", http.StatusNotFound)
}

func isValidLang(lang string) bool {
	for _, l := range languages {
		if l == lang {
			return true
		}
	}
	return false
}

func isAlpha(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}
