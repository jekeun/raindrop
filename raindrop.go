package main

import (
	"fmt"
	"github.com/natefinch/lumberjack"
	"log"
	"os"
	"raindrop/main/model"
	"raindrop/main/strategy"
	printUtil "raindrop/main/utils/print"
	"time"
)

var config *model.Config
var larryRunner *strategy.LarryRunner

func main() {
	fmt.Println("Start RainDrop")

	config = new(model.Config)

	config.LoadConfiguration("./config.json")

	initRaindrop()

	// 10초 단위로 수행
	for {
		runStrategy()
		time.Sleep(10 * time.Second)
	}
}

func initRaindrop() {
	// 처음 실행시키는 경우,
	fmt.Println("Init : Config information")
	fmt.Println(printUtil.PrettyPrint(config))

	f, err := os.OpenFile("./log/raindrop.log",
		os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Println(err)
	}

	logger := log.New(f, "RainDrop : ", log.LstdFlags)
	logger.SetOutput(&lumberjack.Logger{
		Filename:   "./log/raindrop.log",
		MaxSize:    20, // megabytes after which new file is created
		MaxBackups: 5, // number of backups
		MaxAge:     31, //days
	})
	logger.Println("Start raindrop")

	larryRunner = new(strategy.LarryRunner)
	larryRunner.Init(config, logger)
}

func runStrategy() {


	larryRunner.RunLarryStrategy()
}


