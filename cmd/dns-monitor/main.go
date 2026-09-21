package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"dnsmonitor/internal/httpapi"
	"dnsmonitor/internal/model"
	"dnsmonitor/internal/monitor"
	"dnsmonitor/internal/store"
	"dnsmonitor/internal/winservice"
)

type rotatingLog struct {
	mu   sync.Mutex
	path string
	file *os.File
	size int64
}

func newLog(path string) (*rotatingLog, error) {
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return nil, e
	}
	st, e := f.Stat()
	if e != nil {
		f.Close()
		return nil, e
	}
	return &rotatingLog{path: path, file: f, size: st.Size()}, nil
}
func (r *rotatingLog) Write(b []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size+int64(len(b)) > 2<<20 {
		r.file.Close()
		_ = os.Remove(r.path + ".2")
		_ = os.Rename(r.path+".1", r.path+".2")
		_ = os.Rename(r.path, r.path+".1")
		f, e := os.OpenFile(r.path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if e != nil {
			return 0, e
		}
		r.file = f
		r.size = 0
	}
	n, e := r.file.Write(b)
	r.size += int64(n)
	return n, e
}
func (r *rotatingLog) Close() error { r.mu.Lock(); defer r.mu.Unlock(); return r.file.Close() }

// locateDoggo returns the DOGGO executable: the explicit --doggo path when given,
// otherwise doggo.exe beside the binary or on PATH. It does not touch the filesystem
// unless a path search is required, so service control ("service status") never
// depends on a reachable DOGGO.
func locateDoggo(explicit, base string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	local := filepath.Join(base, "doggo.exe")
	if info, err := os.Stat(local); err == nil && !info.IsDir() {
		return local, nil
	}
	if p, err := exec.LookPath("doggo"); err == nil {
		return p, nil
	}
	return "", errors.New("找不到 doggo：未指定 --doggo，且程序目录和 PATH 中均无 doggo.exe。\n请将 doggo.exe 放在 dns-monitor.exe 同目录下，或安装到 PATH 后重试。\n安装方式：\n  scoop install doggo\n  go install github.com/mr-karan/doggo/cmd/doggo@latest\n或从 https://github.com/mr-karan/doggo/releases 下载")
}

func main() {
	if e := entry(); e != nil {
		fmt.Fprintln(os.Stderr, "DNS Monitor:", e)
		os.Exit(1)
	}
}
func entry() error {
	exe, e := os.Executable()
	if e != nil {
		return e
	}
	exe, e = filepath.Abs(exe)
	if e != nil {
		return e
	}
	base := filepath.Dir(exe)
	flags := flag.NewFlagSet("dns-monitor", flag.ContinueOnError)
	dataDir := flags.String("data-dir", filepath.Join(base, "data"), "数据目录（默认程序旁 data）")
	doggoPath := flags.String("doggo", "", "DOGGO 路径（默认先查同目录 doggo.exe，再查 PATH）")
	listen := flags.String("listen", "", "覆盖监听地址（只影响本次启动）")
	open := flags.Bool("open", false, "启动后打开本机网页")
	delay := flags.Duration("helper-delay", 0, "服务辅助进程延迟")
	version := flags.Bool("version", false, "输出版本")
	if e = flags.Parse(os.Args[1:]); e != nil {
		return e
	}
	if *version {
		fmt.Println("DNS Monitor", httpapi.Version)
		return nil
	}
	*dataDir, e = filepath.Abs(*dataDir)
	if e != nil {
		return e
	}
	args := flags.Args()
	if len(args) > 0 && args[0] == "service" {
		if len(args) != 2 {
			return errors.New("用法: dns-monitor.exe service install|uninstall|start|stop|restart|status")
		}
		if args[1] == "status" {
			st := winservice.Inspect()
			fmt.Printf("installed=%t state=%s %s\n", st.Installed, st.State, st.Message)
			return nil
		}
		logger, le := newLog(filepath.Join(filepath.Dir(*dataDir), "logs", "service-action.log"))
		if le != nil {
			return le
		}
		defer logger.Close()
		if *delay > 0 {
			if *delay > 5*time.Second {
				return errors.New("helper delay too large")
			}
			time.Sleep(*delay)
		}
		doggoService := *doggoPath
		var err error
		if args[1] == "install" {
			doggoService, err = locateDoggo(*doggoPath, base)
			if err == nil {
				doggoService, err = filepath.Abs(doggoService)
			}
		}
		if err == nil {
			err = winservice.Manage(args[1], exe, *dataDir, doggoService)
		}
		fmt.Fprintf(logger, "%s action=%s result=%v\n", time.Now().Format(time.RFC3339), args[1], err)
		if err == nil {
			fmt.Println("服务操作完成:", args[1])
		}
		return err
	}
	if len(args) > 0 && args[0] != "service-run" {
		return fmt.Errorf("未知命令: %s", args[0])
	}
	doggo, e := locateDoggo(*doggoPath, base)
	if e != nil {
		return e
	}
	*doggoPath, e = filepath.Abs(doggo)
	if e != nil {
		return e
	}
	isService := winservice.IsService()
	run := func(ctx context.Context) error {
		return runApp(ctx, exe, *dataDir, *doggoPath, *listen, *open && !isService)
	}
	if isService {
		return winservice.Run(run)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	return run(ctx)
}

func runApp(ctx context.Context, exe, dataDir, doggoPath, listenOverride string, openBrowser bool) (runErr error) {
	if e := os.MkdirAll(dataDir, 0755); e != nil {
		return e
	}
	logger, e := newLog(filepath.Join(filepath.Dir(dataDir), "logs", "dns-monitor.log"))
	if e != nil {
		return e
	}
	defer logger.Close()
	log.SetOutput(io.MultiWriter(os.Stdout, logger))
	log.SetFlags(log.LstdFlags)
	if winservice.IsService() {
		log.Print("以 Windows 服务方式运行")
	}
	defer func() {
		if runErr != nil {
			log.Printf("程序运行失败: %v", runErr)
		}
	}()
	lock, e := winservice.AcquireLock(filepath.Join(dataDir, "instance.lock"))
	if e != nil {
		return e
	}
	defer lock.Close()
	st, e := store.Open(filepath.Join(dataDir, "monitor.db"))
	if e != nil {
		return fmt.Errorf("打开数据库失败: %w", e)
	}
	defer st.Close()
	cfg, e := st.GetConfig()
	if e != nil {
		return e
	}
	if e = monitor.ValidateConfig(&cfg); e != nil {
		return fmt.Errorf("配置无效: %w", e)
	}
	listen := cfg.Listen
	if listenOverride != "" {
		candidate := cfg
		candidate.Listen = listenOverride
		if e = monitor.ValidateConfig(&candidate); e != nil {
			return e
		}
		listen = candidate.Listen
	}
	listener, e := net.Listen("tcp4", listen)
	if e != nil {
		return fmt.Errorf("监听 %s 失败，可能已有程序或服务运行: %w", listen, e)
	}
	defer listener.Close()
	if info, err := os.Stat(doggoPath); err != nil || info.IsDir() {
		return fmt.Errorf("找不到 DOGGO: %s", doggoPath)
	}
	mon := monitor.New(st, doggoPath)
	api, e := httpapi.New(st, mon, dataDir, exe, doggoPath, cfg.Listen)
	if e != nil {
		return e
	}
	srv := &http.Server{Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	monitorDone := make(chan struct{})
	go func() { defer close(monitorDone); mon.Run(ctx) }()
	cleanupDone := make(chan struct{})
	go func() {
		defer close(cleanupDone)
		clean := func() {
			if e := st.Cleanup(time.Now().Add(-model.Retention).UnixMilli()); e != nil {
				log.Printf("清理过期数据失败: %v", e)
			}
		}
		clean()
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				clean()
			}
		}
	}()
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Serve(listener) }()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	if host == "0.0.0.0" || host == "" {
		host = "127.0.0.1"
	}
	localURL := "http://" + net.JoinHostPort(host, port)
	log.Printf("DNS Monitor %s 已启动，监听 %s，网页 %s", httpapi.Version, listener.Addr(), localURL)
	log.Printf("访问密钥文件: %s", filepath.Join(dataDir, "access-key.txt"))
	if openBrowser {
		if e := winservice.OpenBrowser(localURL); e != nil {
			log.Printf("打开浏览器失败: %v", e)
		}
	}
	var serveErr error
	select {
	case <-ctx.Done():
	case serveErr = <-errCh:
		cancel()
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if e := srv.Shutdown(shutdownCtx); e != nil {
		_ = srv.Close()
	}
	cancel()
	select {
	case <-monitorDone:
	case <-time.After(10 * time.Second):
		log.Print("等待探测退出超时")
	}
	<-cleanupDone
	log.Print("DNS Monitor 已停止，数据已保存")
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) && !strings.Contains(serveErr.Error(), "closed network connection") {
		return serveErr
	}
	return nil
}
