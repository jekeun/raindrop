package model

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	Account struct {
		Accesskey	string `json:"access_key"`
		SecretKey 	string `json:"secret_key"`
	} `json:"account"`
	LarryStrategy struct {
		Enable 	int `json:"enable"`
		KValue 	float64 	`json:"k_value"`
		Period  int 	`json:"period"`
		StartTime int	`json:"start_time"`
		StopLoss float64	`json:"stop_loss"`
		MaxProfit float64 	`json:"max_profit"`
		MinVariability int `json:"min_variability"`
		OrderAmount float64 `json:"order_amount"`
		MaxCoin int `json:"max_coin"`
		AskPeriodMinute int `json:"ask_period_minute"`
		AskOrderGap int `json:"ask_order_gap"`
		MoneyPlan float64 `json:"money_plan"`
		Targets []string `json:"targets"`
	} `json:"larry_strategy"`
	DayGoldStrategy struct {
		Enable 	int `json:"enable"`
		Period  int 	`json:"period"`
		StopLoss int	`json:"stop_loss"`
		MaxProfit float64 	`json:"max_profit"`
		OrderAmount float64 `json:"order_amount"`
		MaxCoin int `json:"max_coin"`
		AskOrderGap int `json:"ask_order_gap"`
		Targets []string `json:"targets"`
	} `json:"day_gold_strategy"`
}

func (C *Config) LoadConfiguration(file string) {
	configFile, err := os.Open(file)
	defer configFile.Close()
	if err != nil {
		fmt.Println(err.Error())
	}
	jsonParser := json.NewDecoder(configFile)
	_ = jsonParser.Decode(C)
}




