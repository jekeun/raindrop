package strategy

import (
	"fmt"
	"github.com/jekeun/upbit-go"
	"github.com/jekeun/upbit-go/types"
	"github.com/jekeun/upbit-go/util"
	"log"
	"raindrop/main/model"
	upbitTool "raindrop/main/upbit"
	coinUtil "raindrop/main/utils/coin"
	"strconv"
	"strings"
	"time"
)

var gLogger *log.Logger
var gConfig *model.Config

type LarryRunner struct {
	client *upbit.Client
	config *model.Config

}

const ASK_PERIOD_MINUTE = 10		// 10 minute

func (runner *LarryRunner) Init(config *model.Config, logger *log.Logger) {
	runner.client = upbit.NewClient(config.Account.Accesskey, config.Account.SecretKey)
	runner.config = config
	gConfig = config
	gLogger = logger
}

func (runner *LarryRunner) RunLarryStrategy() {
	// Time 체크 : 주어진 시간대 + 10분 이내에 매도 주문을 완성시킨다.
	now := time.Now().UTC()

	if now.Hour() == gConfig.LarryStrategy.StartTime &&
		now.Minute() < ASK_PERIOD_MINUTE {
		runner.runLarryAskStrategy()
	} else {
		runner.runLarryBidStrategy()
	}
}


// 매도 전략
func (runner *LarryRunner)runLarryAskStrategy() {

	balances, ordersMap, _ := runner.getBalanceAndWaitOrders()

	//util.PrintBalance(balances)
	//util.PrintOrdersMap(ordersMap)

	// 매도 가능 잔고가 있는 경우
	// askCoins := getCanAskCoins(balances, ordersMap)

	// 매도 가능 잔고를 구하고, 이중에 Target으로 잡은 코인만 추출한다.
	askCoins := upbitTool.GetBalanceCoinsCanAsk(balances)
	targetAskCoins := checkTargetCoins(runner.config, askCoins)

	if len(targetAskCoins) > 0 {
		// Candle 정보 및 현재가 정보를 가져온다.
		candleMap := runner.getDayCandlesByCoins(targetAskCoins)

		runner.askOrder(targetAskCoins, balances, candleMap)

	} else if len(ordersMap[types.ORDERSIDE_ASK]) > 0 {

		waitOrderCoins := upbitTool.GetCoinsFromOrders(ordersMap[types.ORDERSIDE_ASK])
		waitTargetCoins := checkTargetCoins(runner.config, waitOrderCoins)

		if len(waitTargetCoins) > 0 {
			candleMap := runner.getDayCandlesByCoins(waitTargetCoins)

			runner.forceAskOrder(runner.config, ordersMap[types.ORDERSIDE_ASK], waitTargetCoins, candleMap)
		}

		// 미체결 잔고가 있는 경우,
		fmt.Println(ordersMap[types.ORDERSIDE_ASK])
	} else {

	}


	fmt.Println(askCoins)
}

// 매수 전략
func (runner *LarryRunner) runLarryBidStrategy() {

	balances, ordersMap, _ := runner.getBalanceAndWaitOrders()

	// 잔고에 없고 WaitingOrder가 없는 코인으로 탐색 대상을 잡는다.
	availableCoins := getCheckCoins(runner.config, balances, ordersMap)

	fmt.Println(availableCoins)

	// targetBidCoins := checkTargetCoins(config, availableCoins)

	// Candle 정보 및 현재가 정보를 가져온다.
	candleMap := runner.getDayCandlesByCoins(availableCoins)

	// 전략 수행
	runner.doStrategy(balances, availableCoins, candleMap, ordersMap)
}


func checkTargetCoins(config *model.Config, coins []string) (targetCoins []string) {
	targetCoins = make([]string, 0)

	for _, value := range coins {
		for _, targetCoin := range config.LarryStrategy.Targets {
			if value == targetCoin {
				targetCoins = append(targetCoins, targetCoin)
				continue
			}
		}
	}

	return
}


/*
 * 매도 가능한 코인을 얻어온다.
 * 잔고가 있으며, 현재 미체결 Order가 없는 경우
 */
func getCanAskCoins(balances []*types.Balance, ordersMap map[string][]*types.Order) (coins []string) {
	coins = make([]string, 0)

	for _, value := range balances {
		// 원하는 매도 가능 잔고가 아님.
		if value.Currency == "KRW" {
			continue
		}
		currency := coinUtil.GetMarketFromCurrency(value.Currency, "KRW")

		askOrders := ordersMap[types.ORDERSIDE_ASK]

		bFound := false
		for _, value := range askOrders {
			if value.Market == currency {
				bFound = true
			}
		}

		if !bFound {
			coins = append(coins, currency)
		}
	}

	return
}


func (runner *LarryRunner) getBalanceAndWaitOrders()(balances []*types.Balance, ordersMap map[string][]*types.Order, err error ) {
	// check balance
	balances, err = runner.client.Accounts()

	if err != nil {
		log.Println(err)
		return
	}


	// 미체결 Order 확인
	ordersMap, err = runner.client.OrdersMap("", types.ORDERSTATE_WAIT, 1, types.ORDERBY_DESC)
	if err != nil {
		log.Println(err)
		return
	}

	return
}
/*
 * 주문 가능 원화 잔고를 가져온다.
 */
func getAvailableKrwBalance(balances []*types.Balance) (krwBalance float64) {
	for _, value := range balances {
		if value.Currency == "KRW" {
			f, err := strconv.ParseFloat(value.Balance, 64)

			if err != nil {
				krwBalance = 0.0
			} else {
				krwBalance = f
				return
			}
		}
	}

	return
}

/*
 * 전략 실행
 */
func (runner *LarryRunner) doStrategy(
	balances []*types.Balance,
	availableCoins []string,
	candleMap map[string][]*types.DayCandle,
	orderMap map[string][]*types.Order) (err error) {

	// 주문 가능 잔고 체크
	availableKrwBalance := getAvailableKrwBalance(balances)

	if availableKrwBalance < float64(runner.config.LarryStrategy.OrderAmount) {
		return
	}

	// 잔고에 없는 코인을 기준으로 탐색
	for _, value := range availableCoins {
		candleInfo := candleMap[value]
		rangeValue := candleInfo[1].HighPrice - candleInfo[1].LowPrice

		// 변동성 조건에 해당함.
		bidValue := candleInfo[0].OpeningPrice + rangeValue * runner.config.LarryStrategy.KValue
		if candleInfo[0].TradePrice >= bidValue {
			// 매수 주문 실행해야 함.
			fmt.Println("매수 주문 실행 : " + value)

			priceStr :=  fmt.Sprintf("%.8f", bidValue)
			volumeStr := fmt.Sprintf ("%.8f", runner.config.LarryStrategy.OrderAmount/bidValue)

			fmt.Printf("Price : %s, Volume : %s\n", priceStr, volumeStr)

			bidOrder := types.OrderInfo{
				Identifier: strconv.Itoa(int(util.TimeStamp())),
				Side:       types.ORDERSIDE_BID,
				Market:     value,
				Price:      priceStr,
				Volume:     volumeStr,
				OrdType:    types.ORDERTYPE_LIMIT}

			fmt.Println(bidOrder)

			_, err := runner.client.OrderByInfo(bidOrder)

			if err != nil {
				fmt.Println("주문 에러")
			}
		}
	}

	return
}

/*
 * desCoins 리스트에 coin이 있는지 체크
 */
func isExist(coin string, desCoins []string) (bExist bool) {
	bExist = false

	for _, value := range desCoins {
		if strings.EqualFold(coin, value) {
			bExist = true
			return
		}
	}

	return
}

/*
 * order 목록에 coin 주문이 있는지 확인
 */
func existOrder(coin string, orderMap map[string][]*types.Order, orderSide string) (bExist bool) {
	bExist = false

	bidOrders := orderMap[orderSide]

	for _, value := range bidOrders {
		if strings.EqualFold(value.Market, coin) {
			bExist = true
			return
		}
	}

	return
}

/*
 * 이미 잔고에 없고, 미체결 주문이 들어가지 않은 코인을 가져온다.
 */
func getCheckCoins(config *model.Config, balances []*types.Balance, orderMap map[string][]*types.Order) (checkCoins []string ) {

	balanceMap := upbitTool.GetBalanceMap(balances)
	checkCoins = make([]string, 0)

	for i := 0; i < len(config.LarryStrategy.Targets); i++ {
		coin := config.LarryStrategy.Targets[i]

		if _, exist := balanceMap[coin]; !exist {
			if !existOrder(coin, orderMap, types.ORDERSIDE_BID) {
				checkCoins = append(checkCoins, coin)
			}
		}
	}

	return
}


func (runner *LarryRunner) askOrder(coins []string, balances []*types.Balance, candleMap map[string][]*types.DayCandle) {
	balanceMap := upbitTool.GetBalanceMap(balances)

	fmt.Println(balanceMap)

	for _, value := range coins {

		priceStr :=  fmt.Sprintf("%.8f", candleMap[value][0].TradePrice+200)
		volumeStr :=  balanceMap[value].Balance

		askOrder := types.OrderInfo{
			Identifier: strconv.Itoa(int(util.TimeStamp())),
			Side:       types.ORDERSIDE_ASK,
			Market:     value,
			Price:      priceStr,
			Volume:     volumeStr,
			OrdType:    types.ORDERTYPE_LIMIT}

		fmt.Println(askOrder)

		_, err := runner.client.OrderByInfo(askOrder)

		if err != nil {
			fmt.Println("주문 에러")
		}
	}
}


/*
 * 매도 미체결이 계속 남아있으면 가격을 낮춰서라도 강제 매도
 */
func (runner *LarryRunner) forceAskOrder(config *model.Config, orders[]*types.Order, targetCoins []string, candleMap map[string][]*types.DayCandle) {
	for _, value := range orders {

		if isExist(value.Market, targetCoins) {
			orderTime, _ := time.Parse(time.RFC3339, value.CreatedAt)

			now := time.Now()
			timeGap := now.Sub(orderTime)
			elapsedSeconds := int(timeGap.Seconds())

			if elapsedSeconds > config.LarryStrategy.AskOrderGap  {
				//	client.CancelOrder(value.Uuid)
				fmt.Println("주문 진행 시간 : " + strconv.Itoa(elapsedSeconds) + ", 주문취소 : " + value.Uuid)

				cancelOrder, _ := runner.client.CancelOrder(value.Uuid)

				fmt.Println(cancelOrder)

				upbitTool.AskOrder(runner.client, value.Market, value.Volume, candleMap[value.Market][0])
			}
		}
	}
}

func (runner *LarryRunner) getDayCandlesByCoins(coins []string) (candleMap map[string][]*types.DayCandle) {
	candleMap = make(map[string][]*types.DayCandle)
	for _, coin := range coins {
		candles, err := runner.client.DayCandles(coin, map[string]string{
			"count": "5",
		})
		if err != nil {
			log.Println(err)
			continue
		}

		candleMap[coin] = candles
	}

	return
}

//func getDayCandles(config *model.Config) (candleMap map[string][]*types.DayCandle) {
//
//	candleMap = make(map[string][]*types.DayCandle)
//
//	for i := 0; i < len(config.LarryStrategy.Targets); i++ {
//		coin := config.LarryStrategy.Targets[i]
//		candles, err := client.DayCandles(coin, map[string]string{
//			"count": "5",
//		})
//		if err != nil {
//			log.Println(err)
//			continue
//		}
//
//		candleMap[coin] = candles
//	}
//
//	return
//}
