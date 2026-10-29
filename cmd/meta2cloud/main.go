// meta2cloud 服务入口（对应 Python 版 webui 服务）。
//
// 用法：
//
//	meta2cloud serve    # 启动 Web 服务（默认 0.0.0.0:38765）
//
// 整理一律通过 WebAPI 触发：POST /api/pipeline/trigger（WebUI「一键整理」按钮）。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

// shutdownGrace 收到退出信号后等待在途请求完成的最长时间。
const shutdownGrace = 15 * time.Second

func runServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	host := fs.String("host", "0.0.0.0", "监听地址")
	port := fs.Int("port", 38765, "监听端口")
	root := fs.String("root", "", "项目根目录")
	fs.Parse(args)

	rt := webui.InitRuntime(*root)
	server := webui.NewServer(rt)

	addr := fmt.Sprintf("%s:%d", *host, *port)
	log.Printf("Meta2Cloud v%s Web 服务启动：http://%s", version, addr)

	httpSrv := &http.Server{Addr: addr, Handler: server.Handler()}
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		log.Println("收到退出信号，等待在途请求完成后关闭…")
		ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		defer cancel()
		if err := httpSrv.Shutdown(ctx); err != nil {
			log.Printf("关闭超时，强制断开剩余连接：%v", err)
			_ = httpSrv.Close()
		}
	}()

	err := httpSrv.ListenAndServe()
	// ListenAndServe 返回（信号关闭或异常）后统一释放：先停后台任务，再关存储句柄，
	// 避免 SQLite WAL 残留与 watcher 在库关闭后继续写入
	server.Close()
	rt.Shutdown()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Web 服务异常退出：%v", err)
	}
	log.Println("Meta2Cloud 已退出")
}
