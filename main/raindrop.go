package main

import (
	"fmt"
	"log"
	"os"
	"raindrop/main/model"
	"raindrop/main/strategy"
	printUtil "raindrop/main/utils/print"
	"time"
)

var config *model.Config
var larryRunner *strategy.LarryRunner

var count = 1

type DummyJob struct{}

func (d DummyJob) Run() {

	fmt.Printf("Every %d Seconds\n", count)
	count++
}

func main() {
	fmt.Println("Start RainDrop")

	config = new(model.Config)

	config.LoadConfiguration("./main/config.json")

	initRaindrop()

	// 1초 단위로 수행
	for {
		runStrtegy()
		time.Sleep(5 * time.Second)
	}
}

func fmtTest() {
	fmt.Println(printUtil.PrettyPrint(config))
}

func initRaindrop() {
	// 처음 실행시키는 경우,
	fmt.Println("Init")

	f, err := os.OpenFile("raindrop.log",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println(err)
	}

	logger := log.New(f, "RainDrop : ", log.LstdFlags)
	logger.Println("text to append")
	logger.Println("more text to append")


	larryRunner = new(strategy.LarryRunner)
	larryRunner.Init(config, logger)
}

func runStrtegy() {
	fmt.Println(time.Now())
	larryRunner.RunLarryStrategy()

}


