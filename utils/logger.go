package utils

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// dailyWriter 实现了 io.Writer，每天自动切换日志文件
// 同时写入终端和当天的日志文件
type dailyWriter struct {
	mu   sync.Mutex
	dir  string
	file *os.File
	day  string
	out  io.Writer
}

func (w *dailyWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	today := time.Now().Format("20060102")
	if today != w.day {
		w.rotate(today)
	}

	return w.out.Write(p)
}

func (w *dailyWriter) rotate(today string) {
	if w.file != nil {
		w.file.Close()
	}

	w.day = today
	filename := filepath.Join(w.dir, "app_"+today+".log")
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		// 打开日志文件失败时退化为仅输出到终端
		w.out = os.Stdout
		return
	}
	w.file = f
	// 同时写入终端和日志文件
	w.out = io.MultiWriter(os.Stdout, f)
}

// cleanupOldLogs 清理超过 keepDays 天的旧日志文件
func cleanupOldLogs(dir string, keepDays int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	cutoff := time.Now().AddDate(0, 0, -keepDays)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// 匹配 app_YYYYMMDD.log 格式
		if !strings.HasPrefix(name, "app_") || !strings.HasSuffix(name, ".log") {
			continue
		}
		dateStr := strings.TrimPrefix(name, "app_")
		dateStr = strings.TrimSuffix(dateStr, ".log")
		t, err := time.Parse("20060102", dateStr)
		if err != nil {
			continue
		}
		if t.Before(cutoff) {
			os.Remove(filepath.Join(dir, name))
			log.Printf("清理旧日志文件: %s", name)
		}
	}
}

// InitLogger 初始化日志系统
// 创建 logs 目录，清理旧日志，设置 daily rotating writer
func InitLogger(logDir string) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("创建日志目录失败: %v", err)
		return
	}

	// 清理 5 天前的旧日志
	cleanupOldLogs(logDir, 5)

	dw := &dailyWriter{dir: logDir}
	dw.rotate(time.Now().Format("20060102"))

	// 设置全局 log 的输出目标
	log.SetOutput(dw)
	// 日志格式: 2009/01/23 01:23:23 消息内容
	log.SetFlags(log.LstdFlags)
}
