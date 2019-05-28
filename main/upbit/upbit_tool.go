package upbit

import (
	"fmt"
	"github.com/jekeun/upbit-go"
	"github.com/jekeun/upbit-go/types"
	"github.com/jekeun/upbit-go/util"
	coinUtil "raindrop/main/utils/coin"
	"strconv"
)

func GetBalanceMap(balances []*types.Balance) (balanceMap map[string]*types.Balance) {

	balanceMap = make(map[string]*types.Balance)

	for _, value := range balances {

		currency := coinUtil.GetMarketFromCurrency(value.Currency, "KRW")

		balanceMap[currency] = value
	}

	return
}

func GetCoinsFromOrders(orders []*types.Order) (coins []string) {
	coins = make([]string, 0)

	for _, value := range orders {
		coins = append(coins, value.Market)
	}

	return
}

func GetBalanceCoinsCanAsk(balances []*types.Balance) (coins []string) {
	coins = make([]string, 0)

	for _, value := range balances {
		// 원하는 매도 가능 잔고가 아님.
		if value.Currency == "KRW" {
			continue
		}
		currency := coinUtil.GetMarketFromCurrency(value.Currency, "KRW")

		s, _ := strconv.ParseFloat(value.Balance, 64)

		if s > 0 {
			coins = append(coins, currency)
		}
	}

	return
}

func AskOrder(client *upbit.Client, coin string, balance string, candle *types.DayCandle) {

		priceStr :=  fmt.Sprintf("%.8f", candle.TradePrice+200)
		volumeStr :=  balance

		askOrder := types.OrderInfo{
			Identifier: strconv.Itoa(int(util.TimeStamp())),
			Side:       types.ORDERSIDE_ASK,
			Market:     coin,
			Price:      priceStr,
			Volume:     volumeStr,
			OrdType:    types.ORDERTYPE_LIMIT}

		fmt.Println(askOrder)

		_, err := client.OrderByInfo(askOrder)

		if err != nil {
			fmt.Println("주문 에러")
		}

}

