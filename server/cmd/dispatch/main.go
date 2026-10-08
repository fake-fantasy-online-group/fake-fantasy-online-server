// dispatch 提供服务器列表和游戏数据查询接口。
// 服务器列表通过 HTTP 返回，字段包括 servers/name/host/port/zone/pvp/busy。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

type srv struct {
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
	Zone string `json:"zone"`
	PvP  bool   `json:"pvp"`
	Busy int    `json:"busy"`
}

var (
	addr = flag.String("addr", ":8088", "监听地址")
	host = flag.String("game-host", "127.0.0.1", "下发给客户端的游戏服地址")
	port = flag.Int("game-port", 19000, "下发给客户端的游戏服端口")
	name = flag.String("name", "怀旧一区", "服务器名")
	zone = flag.String("zone", "龙城", "分区名")
	dsn  = flag.String("dsn", defaultDSN(), "幻想数据库使用的 PostgreSQL DSN")
)

func main() {
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var catalog *catalogStore
	var catalogErr error
	if *dsn == "" {
		catalogErr = errors.New("未设置 DATABASE_URL")
	} else {
		catalog, catalogErr = newCatalogStore(ctx, *dsn)
	}
	if catalog != nil {
		defer catalog.Close()
	}
	if catalogErr != nil {
		// 数据库暂不可用时仍提供服务器列表；目录 API 会返回明确的 503。
		log.Printf("幻想数据库暂不可用: %v", catalogErr)
	}

	body := map[string]any{
		"servers": []srv{{Name: *name, Host: *host, Port: *port, Zone: *zone, PvP: false, Busy: 0}},
	}
	raw, _ := json.MarshalIndent(body, "", " ")

	dispatchHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(raw)
	}

	webRoot, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/dispatch.json", dispatchHandler)
	mux.HandleFunc("/api/catalog/items", catalog.itemsHandler)
	mux.HandleFunc("/api/catalog/meta", catalog.metaHandler)
	mux.HandleFunc("/api/catalog/entries", catalog.entriesHandler)
	mux.HandleFunc("/api/catalog/relations", catalog.relationsHandler)
	mux.HandleFunc("/api/gm/accounts", catalog.accountsHandler)
	mux.Handle("/fantasy-db/", workbenchHeaders(http.StripPrefix("/fantasy-db/", http.FileServer(http.FS(webRoot)))))
	mux.HandleFunc("/fantasy-db", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/fantasy-db/", http.StatusTemporaryRedirect)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/fantasy-db/", http.StatusTemporaryRedirect)
			return
		}
		if dispatchCompatibilityPath(r.URL.Path) {
			dispatchHandler(w, r)
			return
		}
		http.NotFound(w, r)
	})

	log.Printf("dispatch 服务已监听 %s", *addr)
	log.Printf("幻想数据库: http://127.0.0.1%s/fantasy-db/", *addr)
	log.Printf("下发内容: %s", raw)
	server := &http.Server{
		Addr:              *addr,
		Handler:           requestLog(mux),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Fatal(server.ListenAndServe())
}

func defaultDSN() string {
	return os.Getenv("DATABASE_URL")
}

func dispatchCompatibilityPath(requestPath string) bool {
	cleanPath := path.Clean(requestPath)
	if strings.HasSuffix(cleanPath, "/dispatch.json") {
		return true
	}
	for _, segment := range strings.Split(strings.Trim(cleanPath, "/"), "/") {
		if strings.HasSuffix(segment, "-private-svcs") {
			return true
		}
	}
	return false
}

func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/catalog/items" {
			log.Printf("%s %s  从 %s", r.Method, r.URL.RequestURI(), r.RemoteAddr)
		}
		next.ServeHTTP(w, r)
	})
}
