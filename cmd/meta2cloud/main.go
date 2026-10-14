// meta2cloud 命令行入口（对应 Python 版 webui 服务 + python -m core 流水线）。
//
// 用法：
//
//	meta2cloud serve                  # 启动 Web 服务（默认 0.0.0.0:38765）
//	meta2cloud pipeline               # 运行整理流水线（等价 python -m core）
//	meta2cloud pipeline --dry-run     # 预览计划，不实际操作
//	meta2cloud pipeline --no-tmdb     # 只整理文件夹，不查 TMDB
//	meta2cloud pipeline --no-images   # 跳过图片下载上传
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"meta2cloud/internal/config"
	"meta2cloud/internal/pipeline"
	"meta2cloud/internal/storage"
	"meta2cloud/internal/webui"
)

// version 构建时由 -ldflags "-X main.version=..." 注入（CI/脚本传入 tag）。
var version = "dev"

const usage = `Meta2Cloud — 扫描云存储媒体文件，查询 TMDB 元数据，生成 NFO，整理到目标文件夹

用法:
  meta2cloud serve [flags]        启动 Web 服务
  meta2cloud pipeline [flags]     运行整理流水线

pipeline flags:
  --dry-run        只打印计划，不实际操作
  --no-tmdb        跳过 TMDB 查询（不生成 NFO，只整理文件夹）
  --no-images      跳过图片下载上传（poster/fanart）
  --storage NAME   存储后端名称（当前支持 pan115），覆盖配置文件 storage.primary
  --config PATH    配置文件路径（默认自动查找 config/config.yaml）

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
	case "pipeline":
		runPipeline(os.Args[2:])
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

func runPipeline(args []string) {
	fs := flag.NewFlagSet("pipeline", flag.ExitOnError)
	dryRun := fs.Bool("dry-run", false, "只打印计划，不实际操作")
	noTmdb := fs.Bool("no-tmdb", false, "跳过 TMDB 查询")
	noImages := fs.Bool("no-images", false, "跳过图片下载上传")
	storageName := fs.String("storage", "", "存储后端名称")
	configPath := fs.String("config", "", "配置文件路径")
	fs.Parse(args)

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Printf("❌  配置加载失败：%v\n", err)
		os.Exit(1)
	}
	if *storageName != "" {
		cfg.Storage.Primary = *storageName
	}

	provider, err := storage.GetProvider(cfg.ActiveStorageName(), cfg)
	if err != nil {
		fmt.Printf("❌  %v\n", err)
		os.Exit(1)
	}
	log.Printf("使用存储后端：%s", provider.ProviderName())

	pipe := pipeline.New(provider, cfg, pipeline.Options{
		DryRun:     *dryRun,
		SkipTmdb:   *noTmdb,
		SkipImages: *noImages,
	})
	summary := pipe.Run()
	fmt.Printf("\n完成：成功 %d / 跳过 %d / 失败 %d\n", summary.OKCount, summary.SkippedCount, summary.FailedCount)
	if summary.FailedCount > 0 {
		os.Exit(1)
	}
}
