package logging

import "log"

func Setup() {
	log.SetFlags(log.LstdFlags)
}

func Info(format string, args ...interface{}) {
	log.Printf("ℹ️  "+format, args...)
}

func Success(format string, args ...interface{}) {
	log.Printf("✅ "+format, args...)
}

func Warn(format string, args ...interface{}) {
	log.Printf("⚠️  "+format, args...)
}

func Error(format string, args ...interface{}) {
	log.Printf("❌ "+format, args...)
}

func Fatal(format string, args ...interface{}) {
	log.Fatalf("❌ "+format, args...)
}
