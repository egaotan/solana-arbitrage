package utils

import (
	"fmt"
	"log"
	"os"
)

func NewLog(dir, name string) *log.Logger {
	fileName := fmt.Sprintf("%s%s.log", dir, name)
	file, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		panic(err)
	}
	log := log.New(file, "", log.LstdFlags|log.Lmicroseconds)
	return log
}
