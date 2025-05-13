package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger 日志记录器
type Logger struct {
	logFile     *os.File
	logger      *log.Logger
	mutex       sync.Mutex
	logDir      string
	currentDate string
}

// NewLogger 创建新的日志记录器
func NewLogger(logDir string) (*Logger, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}

	l := &Logger{
		logDir: logDir,
	}

	if err := l.rotateLogFile(); err != nil {
		return nil, err
	}

	return l, nil
}

// rotateLogFile 轮转日志文件
func (l *Logger) rotateLogFile() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	// 获取当前日期
	currentDate := time.Now().Format("2006-01-02")

	// 如果日期未变或尚未初始化，无需轮转
	if l.currentDate == currentDate && l.logFile != nil {
		return nil
	}

	// 关闭之前的日志文件
	if l.logFile != nil {
		l.logFile.Close()
	}

	// 创建新的日志文件
	logFilePath := filepath.Join(l.logDir, fmt.Sprintf("crawler-%s.log", currentDate))
	f, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	l.logFile = f
	l.logger = log.New(f, "", log.LstdFlags)
	l.currentDate = currentDate

	return nil
}

// Info 记录信息日志
func (l *Logger) Info(format string, v ...interface{}) {
	l.checkRotate()
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.logger.Printf("[INFO] "+format, v...)
	log.Printf(format, v...) // 同时输出到标准输出
}

// Error 记录错误日志
func (l *Logger) Error(format string, v ...interface{}) {
	l.checkRotate()
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.logger.Printf("[ERROR] "+format, v...)
	log.Printf("ERROR: "+format, v...) // 同时输出到标准输出
}

// Debug 记录调试日志
func (l *Logger) Debug(format string, v ...interface{}) {
	l.checkRotate()
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.logger.Printf("[DEBUG] "+format, v...)
}

// checkRotate 检查是否需要轮转日志文件
func (l *Logger) checkRotate() {
	currentDate := time.Now().Format("2006-01-02")
	if l.currentDate != currentDate {
		l.rotateLogFile()
	}
}

// Close 关闭日志记录器
func (l *Logger) Close() {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if l.logFile != nil {
		l.logFile.Close()
	}
}
