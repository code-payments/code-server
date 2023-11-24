package currencyutil

import (
	"fmt"
	"time"

	"github.com/code-payments/code-server/pkg/code/data/currency"
)

func PrintTable(records []*currency.ExchangeRateRecord) {
	fmt.Println("|index|symbol|time|rate|")
	for _, item := range records {
		fmt.Printf("|%d|%s|%s|%f|\n",
			item.Id,
			item.Symbol,
			item.Time.Format(time.RFC3339),
			item.Rate,
		)
	}
}
