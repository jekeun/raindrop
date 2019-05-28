package coin

import "strings"

func GetCurrencyFromMarket(market string) (currency string){
	// "KRW-ATOM" --> "ATOM"
	splitStrs := strings.Split(market, "-")

	if len(splitStrs) >= 2 {
		currency = splitStrs[1]
	}
	return
}

func GetMarketFromCurrency(currency string, marketName string) (market string) {
	// "ATOM" --> "KRW-ATOM"
	market = marketName + "-" + currency

	return
}
