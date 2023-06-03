package main

import (
	"fmt"
	"time"
)
import "github.com/jonnyspicer/mango"

func main() {
	mc := mango.DefaultClientInstance()
	ticker := time.NewTicker(24 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		fmt.Println(ticker.C)
		err := placeRandomBet(mc)
		if err != nil {
			fmt.Println("Error making API request:", err)
			continue
		}
	}
}

func placeRandomBet(mc *mango.Client) error {
	markets, err := mc.GetMarkets(mango.GetMarketsRequest{
		Limit: 100,
	})
	if err != nil {
		return err
	}
	for _, market := range *markets {
		if market.OutcomeType == mango.Binary {
			err = mc.PostBet(mango.PostBetRequest{
				Amount:     1,
				ContractId: market.Id,
				Outcome:    "YES",
			})
			if err != nil {
				return err
			}
			fmt.Printf("placed bet on outcome %v on market ID %v\n", "YES", market.Id)
			break
		}
	}
	return nil
}
