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

/*
 * 매도 가능 잔고
 */
func GetBalanceCoinsCanAsk(balances []*types.Balance) (coins []string) {
	coins = make([]string, 0)

	for _, value := range balances {
		// KRW는 매도 가능 잔고가 아님.
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

func GetPriceCanOrder(price float64) (orderPriceStr string) {

	switch {
	case price >= 2000000:
		orderPriceStr = fmt.Sprintf("%d", int(price/1000) * 1000)
	case price >= 1000000 && price < 2000000:
		orderPriceStr = fmt.Sprintf("%d", int(price/1000) * 1000)
	case price >= 500000 && price < 1000000:
		orderPriceStr = fmt.Sprintf("%d", int(price/100) * 100)
	case price >= 100000 && price < 500000:
		orderPriceStr = fmt.Sprintf("%d", int(price/100) * 100)
	case price >= 10000 && price < 100000:
		orderPriceStr = fmt.Sprintf("%d", int(price/10) * 10)
	case price >= 1000 && price < 10000:
		orderPriceStr = fmt.Sprintf("%d", int(price/10) * 10)
	case price >= 100 && price < 1000:
		orderPriceStr = fmt.Sprintf("%d", int(price))
	case price >= 10 && price < 100:
		orderPriceStr = fmt.Sprintf("%.1f", price)
	case price < 10:
		orderPriceStr = fmt.Sprintf("%.2f", price)
	}

	return
}