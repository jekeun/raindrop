package lw_basic

import (
	"fmt"
	"github.com/jekeun/upbit-go"
	upbitTool "github.com/jekeun/upbit-go/tool"
	"github.com/jekeun/upbit-go/types"
	upbitUtil "github.com/jekeun/upbit-go/util"
	"log"
	"math"
	"raindrop/main/model"
	"strconv"
	"time"
)

/*
 * Larry Williams Basic 변동성 전략 수행
 */

var gLogger *log.Logger
var gConfig *model.Config
var gCurrentMode = BID_MODE

const (
	BID_MODE = 1 + iota
	ASK_MODE
)

type LarryRunner struct {
	client *upbit.Client
}

const healthCheckCoin = "KRW-ETC"

func (runner *LarryRunner) Init(config *model.Config, logger *log.Logger) {
	runner.client = upbit.NewClient(config.Account.Accesskey, config.Account.SecretKey)
	gConfig = config
	gLogger = logger
}

func (runner *LarryRunner) RunLWBasicStrategy() {
	// Time 체크 : 주어진 시간대 + N분(config) 이내에 매도 주문을 완성시킨다.
	// 의도적으로 특정 시간대에 매도만 수행하게 한다.
	now := time.Now().UTC()

	balances, ordersMap, _ := runner.getBalanceAndWaitOrders()

	runner.healthCheck(ordersMap)

	candleMap := upbitTool.GetDayCandlesByCoins(runner.client, gConfig.LarryStrategy.Targets, 20)

	if len(candleMap) == 0 {
		gLogger.Println("캔들 정보 얻어오기에 실패했음.")
		return
	}

	// 스탑로스 or 익절 체크
	// 기본 로직은 StopLoss 및 StopProfit 을 적용하지 않는다.
	// runner.processStop(gConfig.LarryStrategy.StopLoss, balances, ordersMap, candleMap)

	kMap := getKNoiseValueByDay(candleMap)
	malMap := getMovingAverageLineByDay(candleMap)
	malScoreMap := getMalScore(malMap, candleMap)

	// 주어진 시각, 최대 5분간 잔고 매도만 수행한다.
	if now.Hour() == gConfig.LarryStrategy.StartTime &&
		now.Minute() <= gConfig.LarryStrategy.AskPeriodMinute {
		runner.runLarryAskStrategy(balances, ordersMap, candleMap)
		gCurrentMode = ASK_MODE
	} else {
		if gCurrentMode == ASK_MODE {
			runner.forceAskMarketOrder(ordersMap)
		}

		gCurrentMode = BID_MODE

		runner.runLarryBidStrategy(balances, ordersMap, candleMap, kMap, malScoreMap)
	}
}

/*
 * 현재 매도 미체결 내역에 대해 강제 청산을 수행한다.
 */
func (runner *LarryRunner) forceAskMarketOrder(ordersMap map[string][]*types.Order) {
	if askOrders, exist := ordersMap[types.ORDERSIDE_ASK]; exist {
		for _, order := range askOrders {

			upbitTool.CancelOrderAndAskMarketOrder(runner.client, order)
		}
	}
}

/*
 * 실제 주문으로 HealthCheck
 * 안전하게 ETC 100원으로 주문
 */
func (runner *LarryRunner) healthCheck(ordersMap map[string][]*types.Order) {

	var etcOrder *types.Order = nil

	bidOrders := ordersMap[types.ORDERSIDE_BID]

	for _, order := range bidOrders {
		if order.Market == healthCheckCoin {
			etcOrder = order
			break
		}
	}

	//매수 주문이 있다면 취소
	if etcOrder != nil {
		runner.client.CancelOrder(etcOrder.Uuid)
	} else {
		// 매수
		bidOrder := types.OrderInfo{
			Identifier: strconv.Itoa(int(upbitUtil.TimeStamp())),
			Side:       types.ORDERSIDE_BID,
			Market:     healthCheckCoin,
			Price:      "100",
			Volume:     "100",
			OrdType:    types.ORDERTYPE_LIMIT}

		order, err := runner.client.OrderByInfo(bidOrder)

		if err != nil {
			gLogger.Println("주문 에러 ")
		} else {
			if len(order.Uuid) > 0 {
				gLogger.Println("헬스체크 매수 성공 ")
			}
		}

	}
}

// 매도 전략
func (runner *LarryRunner)runLarryAskStrategy(balances []*types.Balance,
	ordersMap map[string][]*types.Order,
	candleMap map[string][]*types.DayCandle) {

	gLogger.Println("[매도 전략 수행중]")

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

	// 매도가능 잔고에서  Target으로 잡은 코인만 추출한다.
	targetAskCoins := checkTargetCoins(gConfig, askCoins)

	if len(targetAskCoins) <= 0 {
		gLogger.Println("매도 가능 타겟 잔고 없음")
	}

	if len(targetAskCoins) > 0 {
		// Candle 정보 및 현재가 정보를 가져온다.
		gLogger.Printf("잔고 매도 전략 수행 %v\n", targetAskCoins)

		// candleMap := runner.getDayCandlesByCoins(targetAskCoins)

		runner.askOrder(targetAskCoins, balances, candleMap)

	} else if len(ordersMap[types.ORDERSIDE_ASK]) > 0 {

		waitOrderCoins := upbitTool.GetCoinsFromOrders(ordersMap[types.ORDERSIDE_ASK])
		waitTargetCoins := checkTargetCoins(gConfig, waitOrderCoins)

		gLogger.Println("미체결 오더 확인 ")
		if len(waitTargetCoins) > 0 {
			// candleMap := runner.getDayCandlesByCoins(waitTargetCoins)

			runner.forceAskOrder(gConfig, ordersMap[types.ORDERSIDE_ASK], waitTargetCoins, candleMap)
		}

	} else {

	}

	gLogger.Println("[매도 전략 수행 종료]")

}

// 매수 전략
func (runner *LarryRunner) runLarryBidStrategy(balances []*types.Balance,
	ordersMap map[string][]*types.Order,
	candleMap map[string][]*types.DayCandle,
	kMap map[string]float64,
	malScoreMap  map[string]float64) {

	gLogger.Println()
	gLogger.Println("[매수 전략 수행중]")

	logBalance(balances)
	logWaitOrders(ordersMap[types.ORDERSIDE_BID])

	// 잔고에 없고 WaitingOrder가 없는 코인으로 탐색 대상을 잡는다.
	availableCoins := getAvailableCoins(gConfig, balances, ordersMap)

	// targetBidCoins := checkTargetCoins(config, availableCoins)

	// 전략 수행
	runner.doStrategy(balances, availableCoins, candleMap, ordersMap, kMap, malScoreMap)

	gLogger.Println("[매수 전략 수행 종료] ")
}

/*
 * 모든 Order를 취소한다.
 */
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
 * 각 코인별 이동평균선 리스트를 가져온다.
 * 3일 평균 이평선 ~ 18일 평균 이평선까지
 */
func getMovingAverageLineByDay(candleMap map[string][]*types.DayCandle) (
	malMap map[string][]float64) {

	malMap = make(map[string][]float64)

	sum := 0.0

	for key, value := range candleMap {
		sum = 0.0
		malSlice := make([]float64, 0, 0)
		if len(value) > 0 {
			for index, candle := range value {
				sum += candle.TradePrice
				if index >=  2 {	// 3일봉부터
					malSlice = append(malSlice, sum/(float64(index+1)))
				}
			}

			malMap[key] = malSlice
		}

	}

	//fmt.Println(malMap)
	return
}

/*
 * 이동평균선 스코어를 계산한다.
 */
func getMalScore(malMap map[string][]float64,
	candleMap map[string][]*types.DayCandle) (
	scoreMap map[string]float64) {

	if len(malMap) <= 0 {
		return
	}

	scoreMap = make(map[string]float64)

	var currentPrice float64
	var score float64
	for key, value := range malMap {
		if candles, exist := candleMap[key]; exist {
			if len(candles) > 0 {
				currentPrice = candles[0].TradePrice
				score = 0.0
				totalCount := len(value)
				for _, malValue := range value {
					if currentPrice > malValue {
						score += 1.0/float64(totalCount)
					}
				}

				score = math.Round(score*100)/100
				scoreMap[key] = score
			}
		}
	}

	return
}
/*
 * K Value 계산
 * 노이즈 비율 = 1-abs(시가-종가)/(고가-저가)
 * k = 최근 20일간의 노이즈 비율의 평균 값
 */
func getKNoiseValueByDay(candleMap map[string][]*types.DayCandle) (
	kMap map[string]float64) {

	kMap = make(map[string]float64)
	var kValue float64
	var kSum float64
	for key, value := range candleMap {
		kValue = 0.0
		kSum = 0.0
		if len(value) > 0 {
			for _, candle := range value {
				candleKValue := 1 - math.Abs((candle.OpeningPrice - candle.TradePrice)/(candle.HighPrice-candle.LowPrice))
				kSum += candleKValue
			}
			kValue = kSum/float64(len(value))

			kMap[key] = kValue
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
		currency := upbitUtil.GetMarketFromCurrency(value.Currency, "KRW")

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


/*
 * 밸런스와 미체결오더를 같이 가져온다.
 */
func (runner *LarryRunner) getBalanceAndWaitOrders()(
	balances []*types.Balance,
	ordersMap map[string][]*types.Order,
	err error ) {
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
 * param
 * balances : 잔고 목록
 * availableCoins : 매매 가능 코인
 * candleMap : 코인별 일봉 캔들 목록
 * orderMap : 현재 걸려있는 미체결 주문 목록
 * kMap : 코인별 K-Value
 * malScoreMap : 코인별 이동평균선 스코어
 */
func (runner *LarryRunner) doStrategy(
	balances []*types.Balance,
	availableCoins []string,
	candleMap map[string][]*types.DayCandle,
	orderMap map[string][]*types.Order,
	kMap map[string]float64,
	malScoreMap map[string]float64) (err error) {

	// 주문 가능 잔고 체크
	availableKrwBalance := getAvailableKrwBalance(balances)

	gLogger.Printf("주문 가능 잔고 : %f\n", availableKrwBalance )

	if availableKrwBalance < float64(gConfig.LarryStrategy.OrderAmount) {
		gLogger.Printf("주문 가능 잔고가 최소 주문 금액보다 적음 : %f\n", availableKrwBalance)
		return
	}

	if len(balances) - 1 >= gConfig.LarryStrategy.MaxCoin {
		gLogger.Printf("기존 보유 코인이 설정값 초과 : 보유코인 %d, 설정값 %d\n",
			len(balances) - 1, gConfig.LarryStrategy.MaxCoin)
		return
	}

	// 잔고에 없는 코인을 기준으로 탐색
	for _, coinName := range availableCoins {
		//candleInfo := candleMap[value]

		if candleInfo, exist := candleMap[coinName]; exist {
			if len(candleInfo) < 2 {	// 최소한 봉이 2개 이상 있어야 판단 가능.
				continue
			}

			// 고가 - 저가 = 범위값
			rangeValue := candleInfo[1].HighPrice - candleInfo[1].LowPrice
			kValue := gConfig.LarryStrategy.KValue

			if coinKValue, valueExist := kMap[coinName]; valueExist {
				kValue = coinKValue
			}

			kAppliedValue := rangeValue * kValue

			gLogger.Printf("==== 전략 수행 코인 : %s ====\n", coinName)
			gLogger.Printf("전일 고가 : %f, 전일 저가 : %f , Range : %f, Range-K value : %f\n",
				candleInfo[1].HighPrice, candleInfo[1].LowPrice, rangeValue, kValue)

			// 변동성 조건에 해당함.
			bidValue := candleInfo[0].OpeningPrice + kAppliedValue

			gLogger.Printf("당일 시가 %f\n", candleInfo[0].OpeningPrice)
			gLogger.Printf("이동평균 Score %f, 변동성 적용 가격 %f\n", malScoreMap[coinName], kAppliedValue)
			gLogger.Printf("매수 조건 가격 %f\n", bidValue)
			gLogger.Printf("현재 가격 %f\n", candleInfo[0].TradePrice)

			if candleInfo[0].TradePrice >= bidValue {
				// 매수 주문 실행.
				// priceStr :=  fmt.Sprintf("%.8f", bidValue)
				priceStr := upbitTool.GetPriceCanOrder(bidValue)

				orderAmount := getOrderAmount(gConfig.LarryStrategy.OrderAmount, malScoreMap, coinName)
				// orderAmount := gConfig.LarryStrategy.OrderAmount * malScoreMap[coinName]
				// volumeStr := fmt.Sprintf("%.8f", gConfig.LarryStrategy.OrderAmount/bidValue)

				if orderAmount <= 0 {
					gLogger.Printf("**** 매수 신호 발생  , 매수 주문 Coin : %s , Price : %s, Volume : 0.0\n",
						coinName, priceStr)
					continue
				}

				volumeStr := fmt.Sprintf("%.8f", orderAmount/bidValue)

				gLogger.Printf("**** 매수 신호 발생  , 매수 주문 Coin : %s , Price : %s, Volume : %s\n",
					coinName, priceStr, volumeStr)

				bidOrder := types.OrderInfo{
					Identifier: strconv.Itoa(int(upbitUtil.TimeStamp())),
					Side:       types.ORDERSIDE_BID,
					Market:     coinName,
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
		}

		gLogger.Printf("\n")
	}

	return
}

func getOrderAmount(defaultAmount float64, malScoreMap map[string]float64, coinName string) (orderAmount float64){

	orderAmount = 0.0
	if malScore, exist := malScoreMap[coinName]; exist {
		orderAmount = defaultAmount * malScore
	}

	return
}
/*
 * 전략 수행에 적용할 코인 목록 가져오기
 * 이미 잔고에 없고, 미체결 주문이 들어가지 않은 코인을 가져온다.
 */
func getAvailableCoins(config *model.Config,
	balances []*types.Balance,
	orderMap map[string][]*types.Order) (
	checkCoins []string ) {

	balanceMap := upbitTool.GetBalanceMap(balances)
	checkCoins = make([]string, 0)

	for i := 0; i < len(config.LarryStrategy.Targets); i++ {
		coin := config.LarryStrategy.Targets[i]

		if _, exist := balanceMap[coin]; !exist {
			if _, exist := upbitTool.ExistOrder(coin, orderMap, types.ORDERSIDE_BID); !exist {
				checkCoins = append(checkCoins, coin)
			}
		}
	}
	return
}

/*
 * coins 의 코인들이 balances 에 있으면 현재가로 매도 수행
 */
func (runner *LarryRunner) askOrder(coins []string,
	balances []*types.Balance,
	candleMap map[string][]*types.DayCandle) {

	balanceMap := upbitTool.GetBalanceMap(balances)

	for _, value := range coins {
		priceStr :=  fmt.Sprintf("%.8f", candleMap[value][0].TradePrice)
		volumeStr :=  balanceMap[value].Balance

		gLogger.Printf("매도 주문 수행 %s : 가격 , %s , 수량 , %s", value, priceStr, volumeStr)

		askOrder := types.OrderInfo{
			Identifier: strconv.Itoa(int(upbitUtil.TimeStamp())),
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

	gLogger.Println("매도 가격 조정 수행")

	for _, value := range orders {

		if upbitTool.IsExist(value.Market, targetCoins) {
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
					upbitTool.AskOrder(runner.client, value.Market, value.Volume, candleMap[value.Market][0],  types.ORDERTYPE_LIMIT)
				}
			}
		}
	}
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

/*
 * StopLoss or ProfitLoss 프로세스 실행
 */

func (runner *LarryRunner) processStop(stopLossRate float64,
	balances []*types.Balance,
	ordersMap map[string][]*types.Order,
	candleMap map[string][]*types.DayCandle) {

	if len(balances) <= 0 {
		return
	}

	for _, balance := range balances {
		if balance.Currency == "KRW" {
			continue
		}

		coinStr := upbitUtil.GetMarketFromCurrency(balance.Currency, "KRW")

		currentPrice := upbitTool.GetCurrentPriceFromDayCandle(candleMap[coinStr])

		profitRate := upbitTool.GetProfitRate(balance, currentPrice)

		//profitRate = -10

		if stopLossRate > profitRate {
			if order, exist := upbitTool.ExistOrder(coinStr, ordersMap, types.ORDERSIDE_ASK); exist {
				runner.client.CancelOrder(order.Uuid)
			}

			// Ask Order
			upbitTool.AskMarketOrder(runner.client, coinStr, balance.Balance)


		}
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
