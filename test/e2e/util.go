package e2e

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/onsi/ginkgo"
)

func nowStamp() string {
	return time.Now().Format(time.RFC3339)
}

func log(level string, format string, args ...interface{}) {
	fmt.Fprintf(ginkgo.GinkgoWriter, nowStamp()+": "+level+": "+format+"\n", args...)
}

// Logf logs in e2e framework
func Logf(format string, args ...interface{}) {
	log("INFO", format, args...)
}

// Failf reports a failure in the current e2e
func Failf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	log("ERROR", msg)
	ginkgo.Fail(nowStamp()+": "+msg, 1)
}

// LogAndReturnErrorf logs and return an error
func LogAndReturnErrorf(format string, args ...interface{}) error {
	Logf(format, args...)
	return fmt.Errorf(format, args...)
}

var letters = []rune("abcdefghijklmnopqrstuvwxyz")

// RandString create a random string
func RandString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
