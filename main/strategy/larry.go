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
}

const ASK_PERIOD_MINUTE = 10		// 10 minute

func (runner *LarryRunner) Init(config *model.Config, logger *log.Logger) {
	runner.client = upbit.NewClient(config.Account.Accesskey, config.Account.SecretKey)
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

	gLogger.Println("[매도 전략 수행중]")
	balances, ordersMap, _ := runner.getBalanceAndWaitOrders()

	logBalance(balances)
	logWaitOrders(ordersMap[types.ORDERSIDE_ASK])

	//util.PrintBalance(balances)
	//util.PrintOrdersMap(ordersMap)

	// 매도 가능 잔고가 있는 경우
	// askCoins := getCanAskCoins(balances, ordersMap)

	// 이 시간대 매수 오더는 전일 오더이므로 모두 취소시킨다.
	bidOrders := ordersMap[types.ORDERSIDE_BID]
	if len(bidOrders) > 0 {
		runner.cancelAllOrder(bidOrders)
	}


	// 매도 가능 잔고를 구함 ,
	askCoins := upbitTool.GetBalanceCoinsCanAsk(balances)
	if len(askCoins) > 0 {
		gLogger.Printf("매도 가능 잔고 : %v\n", askCoins)
	} else {
		gLogger.Println("매도 가능 잔고 없음")
	}

	// 이중에 Target으로 잡은 코인만 추출한다.
	targetAskCoins := checkTargetCoins(gConfig, askCoins)

	if len(targetAskCoins) <= 0 {
		gLogger.Println("매도 가능 타겟 잔고 없음")
	}

	if len(targetAskCoins) > 0 {
		// Candle 정보 및 현재가 정보를 가져온다.
		gLogger.Printf("잔고 매도 전략 수행 %v\n", targetAskCoins)

		candleMap := runner.getDayCandlesByCoins(targetAskCoins)

		runner.askOrder(targetAskCoins, balances, candleMap)

	} else if len(ordersMap[types.ORDERSIDE_ASK]) > 0 {

		waitOrderCoins := upbitTool.GetCoinsFromOrders(ordersMap[types.ORDERSIDE_ASK])
		waitTargetCoins := checkTargetCoins(gConfig, waitOrderCoins)

		gLogger.Println("미체결 오더 확인 ")
		if len(waitTargetCoins) > 0 {
			candleMap := runner.getDayCandlesByCoins(waitTargetCoins)

			runner.forceAskOrder(gConfig, ordersMap[types.ORDERSIDE_ASK], waitTargetCoins, candleMap)
		}

	} else {

	}

	gLogger.Println("[매도 전략 수행 종료]")

}

// 매수 전략
func (runner *LarryRunner) runLarryBidStrategy() {

	gLogger.Println()
	gLogger.Println("[매수 전략 수행중]")
	balances, ordersMap, _ := runner.getBalanceAndWaitOrders()

	logBalance(balances)
	logWaitOrders(ordersMap[types.ORDERSIDE_BID])

	// 잔고에 없고 WaitingOrder가 없는 코인으로 탐색 대상을 잡는다.
	availableCoins := getCheckCoins(gConfig, balances, ordersMap)

	// targetBidCoins := checkTargetCoins(config, availableCoins)

	// Candle 정보 및 현재가 정보를 가져온다.
	candleMap := runner.getDayCandlesByCoins(availableCoins)

	// 전략 수행
	runner.doStrategy(balances, availableCoins, candleMap, ordersMap)

	gLogger.Println("[매수 전략 수행 종료] ")
}


func (runner *LarryRunner) cancelAllOrder(orders []*types.Order) {

	for _, value := range orders {
		order, _ := runner.client.CancelOrder(value.Uuid)
		gLogger.Printf("주문 취소 : %s, %s", order.Uuid, order.Side)
	}
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

	gLogger.Printf("주문 가능 잔고 : %f\n", availableKrwBalance )

	if availableKrwBalance < float64(gConfig.LarryStrategy.OrderAmount) {
		gLogger.Printf("주문 가능 잔고가 최소 주문 금액보다 적음 : %f\n", availableKrwBalance)
		return
	}

	if len(balances) - 1 > gConfig.LarryStrategy.MaxCoin {
		gLogger.Printf("기존 보유 코인이 설정값 초과 : 보유코인 %d, 설정값 %d\n", len(balances) - 1, gConfig.LarryStrategy.MaxCoin)
		return
	}


	// 잔고에 없는 코인을 기준으로 탐색
	for _, value := range availableCoins {
		candleInfo := candleMap[value]
		rangeValue := candleInfo[1].HighPrice - candleInfo[1].LowPrice
		kValue := rangeValue * gConfig.LarryStrategy.KValue
		gLogger.Printf("==== 전략 수행 코인 : %s ====\n", value)
		gLogger.Printf("전일 고가 : %f, 전일 저가 : %f , Range : %f, Range-K value : %f\n", candleInfo[1].HighPrice, candleInfo[1].LowPrice, rangeValue, kValue)

		// 변동성 조건에 해당함.
		bidValue := candleInfo[0].OpeningPrice + kValue

		gLogger.Printf("당일 시가 %f\n", candleInfo[0].OpeningPrice)
		gLogger.Printf("매수 조건 가격 %f\n", bidValue)
		gLogger.Printf("현재 가격 %f\n", candleInfo[0].TradePrice)

		if candleInfo[0].TradePrice >= bidValue {
			// 매수 주문 실행.
			// priceStr :=  fmt.Sprintf("%.8f", bidValue)
			priceStr := upbitTool.GetPriceCanOrder(bidValue)
			volumeStr := fmt.Sprintf ("%.8f", gConfig.LarryStrategy.OrderAmount/bidValue)


			gLogger.Printf("**** 매수 신호 발생  , 매수 주문 Coin : %s , Price : %s, Volume : %s\n", value, priceStr, volumeStr)

			bidOrder := types.OrderInfo{
				Identifier: strconv.Itoa(int(util.TimeStamp())),
				Side:       types.ORDERSIDE_BID,
				Market:     value,
				Price:      priceStr,
				Volume:     volumeStr,
				OrdType:    types.ORDERTYPE_LIMIT}


			order, err := runner.client.OrderByInfo(bidOrder)

			if err != nil {
				gLogger.Println("주문 에러 ")
			} else {
				if len(order.Uuid) > 0 {
					gLogger.Println("매수 성공 ")
					gLogger.Printf("코인 %s, 주문가격 : %s, 주문수량 :%s", order.Market, order.Price, order.Volume)
				}
			}
		} else {
			gLogger.Printf("==== 매수 신호 대기 ====\n")
		}

		gLogger.Printf("\n")
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

	for _, value := range coins {
		priceStr :=  fmt.Sprintf("%.8f", candleMap[value][0].TradePrice)
		volumeStr :=  balanceMap[value].Balance

		gLogger.Printf("매도 주문 수행 %s : 가격 , %s , 수량 , %s", value, priceStr, volumeStr)

		askOrder := types.OrderInfo{
			Identifier: strconv.Itoa(int(util.TimeStamp())),
			Side:       types.ORDERSIDE_ASK,
			Market:     value,
			Price:      priceStr,
			Volume:     volumeStr,
			OrdType:    types.ORDERTYPE_LIMIT}

		_, err := runner.client.OrderByInfo(askOrder)

		if err != nil {
			// fmt.Println("주문 에러")
			gLogger.Println("주문 에러")
		} else {
			gLogger.Println("주문 성공")
		}
	}
}


/*
 * 매도 미체결이 계속 남아있으면 가격을 낮춰서라도 강제 매도
 */
func (runner *LarryRunner) forceAskOrder(config *model.Config, orders[]*types.Order, targetCoins []string, candleMap map[string][]*types.DayCandle) {

	gLogger.Println("강제 매도 수행")
	for _, value := range orders {

		if isExist(value.Market, targetCoins) {
			orderTime, _ := time.Parse(time.RFC3339, value.CreatedAt)

			now := time.Now()
			timeGap := now.Sub(orderTime)
			elapsedSeconds := int(timeGap.Seconds())

			if elapsedSeconds > config.LarryStrategy.AskOrderGap  {
				//	client.CancelOrder(value.Uuid)
				fmt.Println("주문 진행 시간 : " + strconv.Itoa(elapsedSeconds) + ", 주문취소 : " + value.Uuid)
				gLogger.Println(value.Market + ", 주문 진행 시간 : " + strconv.Itoa(elapsedSeconds) + ", 주문취소 : " + value.Uuid)

				_, err := runner.client.CancelOrder(value.Uuid)

				if err != nil {
					gLogger.Printf("주문 취소 실패 : %s\n" + err.Error())
				} else {

					gLogger.Println("매도 주문 실행 ")
					gLogger.Printf("코인 : %s, 주문수량 : %s, 주문가격 : %f\n", value.Market, value.Volume, candleMap[value.Market][0].TradePrice)
					upbitTool.AskOrder(runner.client, value.Market, value.Volume, candleMap[value.Market][0])
				}
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

/*
 * 밸런스 로깅
 */
func logBalance(balances []*types.Balance) {
	gLogger.Println("[잔고 현황] ")
	for _, value := range balances {
		gLogger.Printf("코인 : %s, 잔고 : %s\n", value.Currency, value.Balance)
	}
}

/*
 * 미체결 오더 잔고 로깅
 */
func logWaitOrders(orders []*types.Order) {
	gLogger.Println("[미체결 주문 현황] ")
	if len(orders) >  0 {
		for _, value := range orders {
			gLogger.Printf("코인 : %s, 주문량 %s, 주문가격 %s\n", value.Market, value.Volume, value.Price)
		}
	} else {
		gLogger.Println("미체결 주문 없음")
	}
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
