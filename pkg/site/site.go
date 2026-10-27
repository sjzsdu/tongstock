// Package site 把官网与文档的静态产物嵌进二进制。
//
// 与 pkg/web 完全同款：`make cli`（或 `make site`）会先构建 site/dist 再拷进
// pkg/site/dist，由 //go:embed 打进二进制，因此**运行时不需要 Node**。
package site

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// DistFS 返回站点的只读文件系统（根为 dist 目录）。
func DistFS() http.FileSystem {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err)
	}
	return http.FS(sub)
}

// DistFileServer 返回直接服务站点静态资源的 handler。
func DistFileServer() http.Handler {
	return http.FileServer(DistFS())
}

// Exists 判断站点产物里是否存在该路径（文件或目录）。
// 目录也会返回 true：调用方交给 http.FileServer 处理时会自动补尾斜杠并取 index.html。
//
// 尾斜杠要先去掉：embed.FS 不接受 "dist/docs/" 这种以分隔符结尾的路径（fs.ValidPath 要求），
// 而 HTTP 请求里 /docs/ 很常见，不归一化就会把真实存在的目录判成不存在。
func Exists(path string) bool {
	clean := strings.Trim(path, "/")
	if clean == "" {
		clean = "."
	}
	if !fs.ValidPath(clean) {
		return false
	}
	f, err := distFS.Open("dist/" + clean)
	if err != nil {
		return false
	}
	f.Close()
	return true
}
