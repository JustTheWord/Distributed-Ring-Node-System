package utils

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"
)

var fileMu sync.Mutex

// LogEvent increments the Lamport clock, logs to stdout and a file.
func LogEvent(clock *int, nodeID, event string) {
	incrementLamport(clock)
	ts := *clock
	msg := fmt.Sprintf("[%d][%s] %s", ts, nodeID, event)
	log.Println(msg)

	fileMu.Lock()
	defer fileMu.Unlock()
	f, _ := os.OpenFile("distributed.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	defer f.Close()
	f.WriteString(time.Now().Format(time.RFC3339) + " " + msg + "\n")
}

func TimeNowMillis() int64 {
	return time.Now().UnixNano() / 1_000_000
}

func incrementLamport(clock *int) {
	*clock++
}
