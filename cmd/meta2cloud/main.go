// meta2cloud 服务入口（对应 Python 版 webui 服务）。
//
// 用法：
//
//	meta2cloud serve    # 启动 Web 服务（默认 0.0.0.0:38765）
//
// 整理一律通过 WebAPI 触发：POST /api/pipeline/trigger（WebUI「一键整理」按钮）。
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"meta2cloud/internal/webui"
)

// version 构建时由 -ldflags "-X main.version=..." 注入（CI/脚本传入 tag）。
var version = "dev"

const usage = `Meta2Cloud — 115 网盘影音管理平台：自动整理、资源检索订阅、STRM 直链播放、观看同步

用法:
  meta2cloud serve [flags]        启动 Web 服务（整理通过 WebAPI 触发）

serve flags:
  --host HOST      监听地址（默认 0.0.0.0）
  --port PORT      监听端口（默认 38765）
  --root DIR       项目根目录（默认当前目录）
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(0)
	}
	switch os.Args[1] {
	case "serve":
		runServe(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "未知命令：%s\n\n", os.Args[1])
		fmt.Print(usage)
		os.Exit(2)
	}
}

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	host := fs.String("host", "0.0.0.0", "监听地址")
	port := fs.Int("port", 38765, "监听端口")
	root := fs.String("root", "", "项目根目录")
	fs.Parse(args)

	webui.InitRuntime(*root)
	server := webui.NewServer()
	defer server.Close()

	addr := fmt.Sprintf("%s:%d", *host, *port)
	log.Printf("Meta2Cloud v%s Web 服务启动：http://%s", version, addr)
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		log.Println("收到退出信号")
		os.Exit(0)
	}()
	if err := webui.ListenAndServe(addr, server); err != nil {
		log.Fatalf("Web 服务异常退出：%v", err)
	}
}
